// internal/booking/scenarios_test.go
package booking_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Amireallycodes/Ticket_Booking_System/internal/booking"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://booking:booking@localhost:5433/booking?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return pool
}

// seedSeats creates one movie/screening and n fresh seats, resetting any
// leftover state so each test run starts clean regardless of prior runs.
func seedSeats(t *testing.T, pool *pgxpool.Pool, n int) (screeningID int64, seatIDs []int64) {
	ctx := context.Background()

	var movieID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO movies (name, duration_minutes) VALUES ('Load Test Movie', 120) RETURNING id`,
	).Scan(&movieID)
	if err != nil {
		t.Fatalf("seed movie: %v", err)
	}

	err = pool.QueryRow(ctx,
		`INSERT INTO screenings (movie_id, starts_at) VALUES ($1, now() + interval '1 day') RETURNING id`,
		movieID,
	).Scan(&screeningID)
	if err != nil {
		t.Fatalf("seed screening: %v", err)
	}

	for i := 0; i < n; i++ {
		var seatID int64
		label := fmt.Sprintf("S%d", i+1)
		err = pool.QueryRow(ctx,
			`INSERT INTO seats (screening_id, label, status) VALUES ($1, $2, 'available') RETURNING id`,
			screeningID, label,
		).Scan(&seatID)
		if err != nil {
			t.Fatalf("seed seat: %v", err)
		}
		seatIDs = append(seatIDs, seatID)
	}
	return screeningID, seatIDs
}

// Tests Begin Here
// TestManyUsersSameSeat checks that when many people try to grab the
// same seat at the same time, only one of them succeeds.
func TestManyUsersSameSeat(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	_, seats := seedSeats(t, pool, 1)
	seatID := seats[0]

	store := booking.NewStore(pool)
	numUsers := 200

	var successes int64
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			err := store.HoldSeat(context.Background(), seatID, userID, 5*time.Minute)
			if err == nil {
				atomic.AddInt64(&successes, 1)
			}
		}(int64(i + 1))
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("%d users tried to hold the same seat, %d succeeded, took %v", numUsers, successes, elapsed)
	if successes != 1 {
		t.Errorf("expected only 1 user to get the seat, got %d", successes)
	}
}

// TestManyUsersManySeats checks that when lots of people book different
// seats at the same time, every seat still only goes to one person, and
// no one accidentally takes someone else's seat.
func TestManyUsersManySeats(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	numSeats := 50
	_, seats := seedSeats(t, pool, numSeats)
	store := booking.NewStore(pool)

	numRequests := 500
	var successes int64
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seatID := seats[i%numSeats]
			userID := int64(i + 1)
			err := store.HoldSeat(context.Background(), seatID, userID, 5*time.Minute)
			if err == nil {
				atomic.AddInt64(&successes, 1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	perSecond := float64(numRequests) / elapsed.Seconds()
	t.Logf("%d requests across %d seats, %d succeeded, took %v (%.1f requests/sec)",
		numRequests, numSeats, successes, elapsed, perSecond)

	if successes != int64(numSeats) {
		t.Errorf("expected exactly %d successful holds (one per seat), got %d", numSeats, successes)
	}
}

// TestOnlyHolderCanConfirm checks that a user can only confirm a seat
// if they actually held it first — someone else can't jump in and
// confirm a seat they never reserved.
func TestOnlyHolderCanConfirm(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	screeningID, seats := seedSeats(t, pool, 1)
	seatID := seats[0]
	store := booking.NewStore(pool)
	ctx := context.Background()

	// User 1 holds the seat, then confirms it — this should work.
	if err := store.HoldSeat(ctx, seatID, 1, 5*time.Minute); err != nil {
		t.Fatalf("user 1 couldn't hold the seat: %v", err)
	}
	if err := store.ConfirmBooking(ctx, seatID, 1, screeningID); err != nil {
		t.Errorf("user 1 should be able to confirm, got error: %v", err)
	}

	// On a different, fresh seat: user 2 tries to confirm without
	// ever holding it — this should fail.
	_, moreSeats := seedSeats(t, pool, 1)
	otherSeatID := moreSeats[0]
	err := store.ConfirmBooking(ctx, otherSeatID, 2, screeningID)
	if err == nil {
		t.Error("expected an error — user 2 never held this seat")
	}
	t.Logf("correctly blocked: %v", err)
}

// TestExpiredHoldFreesTheSeat checks that if someone holds a seat but
// doesn't confirm it in time, another person can grab it after the
// hold runs out.
func TestExpiredHoldFreesTheSeat(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	_, seats := seedSeats(t, pool, 1)
	seatID := seats[0]
	store := booking.NewStore(pool)
	ctx := context.Background()

	// User 1 holds for just 2 seconds.
	if err := store.HoldSeat(ctx, seatID, 1, 2*time.Second); err != nil {
		t.Fatalf("user 1 couldn't hold the seat: %v", err)
	}

	// User 2 tries right away — should fail, the hold is still active.
	if err := store.HoldSeat(ctx, seatID, 2, 5*time.Minute); err == nil {
		t.Error("user 2 should NOT be able to hold yet — user 1's hold is still active")
	}

	time.Sleep(2500 * time.Millisecond) // wait for user 1's hold to expire

	// Now user 2 tries again — should succeed.
	if err := store.HoldSeat(ctx, seatID, 2, 5*time.Minute); err != nil {
		t.Errorf("user 2 should be able to hold after the expiry, got error: %v", err)
	}
	t.Log("expired hold correctly freed up the seat")
}

//To run this test file:
//$env:DATABASE_URL = "postgres://booking:booking@localhost:5433/booking?sslmode=disable"
//go test -v -count=1 ./internal/booking/...
//OR
//go test -v -count=1 ./internal/booking/... -run "TestManyUsersSameSeat|TestManyUsersManySeats|TestOnlyHolderCanConfirm|TestExpiredHoldFreesTheSeat"
