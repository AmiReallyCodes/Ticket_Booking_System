package booking

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Movie struct {
	MovieID         int64  `json:"movie_id"`
	MovieName       string `json:"movie_name"`
	Description     string `json:"description"`
	DurationMinutes int    `json:"duration_minutes"`
}

type Screening struct {
	ScreeningID int64     `json:"screening_id"`
	MovieID     int64     `json:"movie_id"`
	StartsAt    time.Time `json:"starts_at"`
}

type Seat struct {
	SeatID      int64              `json:"seat_id"`
	Label       string             `json:"label"`
	ScreeningID int64              `json:"screening_id"`
	Status      SeatStatus         `json:"status"`
	HeldBy      pgtype.Int8        `json:"held_by"`
	HeldUntil   pgtype.Timestamptz `json:"held_until"`
}

// using pgtype - it gives a struct containing var of that type and valid bool.
// if the var is null then Valid is false which replaes null pointer reference check each time or any other stuff
func (s Seat) IsHeld() bool {
	return s.HeldBy.Valid && s.HeldUntil.Valid && s.HeldUntil.Time.After(time.Now())
}

func (s Seat) HeldByUser(userID int64) bool {
	return s.HeldBy.Valid && s.HeldBy.Int64 == userID &&
		s.HeldUntil.Valid && s.HeldUntil.Time.After(time.Now())
}

type Booking struct {
	BookingID   int64         `json:"booking_id"`
	ScreeningID int64         `json:"screening_id"`
	UserID      int64         `json:"user_id"`
	SeatID      int64         `json:"seat_id"`
	CreatedAt   time.Time     `json:"created_at"`
	BStatus     BookingStatus `json:"b_status"`
}

type BookingStatus string

const (
	BookingConfirmed BookingStatus = "confirmed"
	BookingCancelled BookingStatus = "cancelled"
)

type SeatStatus string

const (
	SeatAvailable SeatStatus = "available"
	SeatBooked    SeatStatus = "booked"
)

type BookingStore interface {
	GetSeat(ctx context.Context, seatID int64) (Seat, error)
	MarkSeatBooked(ctx context.Context, seatID int64) error
	MarkSeatAvailable(ctx context.Context, seatID int64) error
	CreateBooking(ctx context.Context, b Booking) error
	ListBookings(ctx context.Context, screeningID int64) ([]Booking, error)
	HoldSeat(ctx context.Context, seatID, userID int64, duration time.Duration) error
	ConfirmBooking(ctx context.Context, seatID, userID, screeningID int64) error
}
