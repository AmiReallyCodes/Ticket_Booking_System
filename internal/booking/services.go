package booking

import (
	"context"
	"time"
)

type Service struct {
	store BookingStore
}

func NewService(store BookingStore) *Service { return &Service{store: store} }

func (svc *Service) HoldSeat(ctx context.Context, seatID, userID int64, duration time.Duration) error {
	return svc.store.HoldSeat(ctx, seatID, userID, duration)
}

func (svc *Service) BookSeat(ctx context.Context, seatID, userID int64) error {
	var seat Seat
	var err error
	seat, err = svc.store.GetSeat(ctx, seatID)
	if err != nil {
		return err
	}

	err = svc.store.ConfirmBooking(ctx, seatID, userID, seat.ScreeningID)
	if err != nil {
		return err
	}
	return nil
}
