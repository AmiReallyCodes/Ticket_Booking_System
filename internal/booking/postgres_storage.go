package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSeatNotFound    = errors.New("seat not found")
	ErrSeatUnavailable = errors.New("seat unavailable")
	ErrNoActiveHold    = errors.New("no active hold for this user")
	ErrHoldExpired     = errors.New("hold has expired")
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// check whether a specific type correctly implements an interface.
// if not it gives compile time error to notify.
var _ BookingStore = (*Store)(nil)

func (s *Store) GetSeat(ctx context.Context, seatID int64) (Seat, error) {
	var seat Seat
	err := s.pool.QueryRow(ctx, `SELECT id, screening_id, label, status, held_by, held_until FROM seats WHERE id = $1`, seatID).Scan(&seat.SeatID, &seat.ScreeningID, &seat.Label, &seat.Status, &seat.HeldBy, &seat.HeldUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Seat{}, ErrSeatNotFound
		}
		return Seat{}, err
	}
	return seat, nil
}

func (s *Store) HoldSeat(ctx context.Context, seatID, userID int64, duration time.Duration) error {
	tag, err := s.pool.Exec(ctx, `UPDATE seats
	 SET held_by =$1 , held_until= now() +$2::interval
	 WHERE id =$3 
	 AND status = 'available' 
     AND (held_until IS NULL OR held_until < now())`, userID, duration, seatID)

	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSeatUnavailable
	}
	return nil
}

func (s *Store) MarkSeatBooked(ctx context.Context, seatID int64) error {
	//didn't eliminate the race condition, just moved the point of truth to somewhere the race can't cause damage.
	//only once the seat will get booked
	tag, err := s.pool.Exec(ctx, `UPDATE seats SET status = 'booked' WHERE id = $1 AND status = 'available'`, seatID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSeatUnavailable
	}
	return nil
}

func (s *Store) MarkSeatAvailable(ctx context.Context, seatID int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE seats SET status = 'available' WHERE id = $1`, seatID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSeatUnavailable
	}
	return nil
}

func (s *Store) CreateBooking(ctx context.Context, b Booking) error {
	tag, err := s.pool.Exec(ctx, `INSERT INTO bookings (screening_id, seat_id, user_id) VALUES ($1, $2, $3)`, b.ScreeningID, b.SeatID, b.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("create booking failed")
	}
	return nil
}
func (s *Store) ListBookings(ctx context.Context, screeningID int64) ([]Booking, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, screening_id, seat_id, user_id, status, created_at FROM bookings WHERE screening_id = $1 ORDER BY id`, screeningID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking

	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.BookingID, &b.ScreeningID, &b.SeatID, &b.UserID, &b.BStatus, &b.CreatedAt); err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bookings, nil
}

func (s *Store) ConfirmBooking(ctx context.Context, seatID, userID, screeningID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// // for showing correct errors... split the query
	// tag, err := tx.Exec(ctx,
	// 	`UPDATE seats
	//  SET status = 'booked', held_by = null, held_until = null
	//  WHERE id = $1
	//  AND status = 'available'
	//  AND held_by=$2
	//  AND held_until > now()`,
	//  seatID, userID,)
	// if err != nil {
	// 	return err
	// }
	// if tag.RowsAffected() == 0 {
	// 	return ErrSeatUnavailable
	// }

	var status string
	var heldBy pgtype.Int8
	var heldUntil pgtype.Timestamptz

	//FOR UPDATE maintains the lock to avoid someone else altering the same row.
	err = tx.QueryRow(ctx, `SELECT status, held_by, held_until FROM seats where id=$1 FOR UPDATE`, seatID).Scan(&status, &heldBy, &heldUntil)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSeatNotFound
		}
		return err
	}

	switch {
	case status != "available":
		return ErrSeatUnavailable
	case !heldBy.Valid || heldBy.Int64 != userID:
		return ErrNoActiveHold
	case !heldUntil.Valid || heldUntil.Time.Before(time.Now()):
		return ErrHoldExpired
	}

	_, err = tx.Exec(ctx,
		`UPDATE seats SET status = 'booked', held_by = NULL, held_until = NULL WHERE id = $1`,
		seatID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `INSERT INTO bookings (screening_id, seat_id, user_id) VALUES ($1, $2, $3)`, screeningID, seatID, userID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
