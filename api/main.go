package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/Amireallycodes/Ticket_Booking_System/internal/booking"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	svc *booking.Service
}

func setUpRoutes(app *fiber.App, h *Handler) {
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Ticket Booking App is running")
	})

	app.Post("/api/seats/:seatID/hold", h.HoldSeat)
	app.Post("/api/seats/:seatID/confirm", h.ConfirmBooking)

}

func ParseSeatRequest(c *fiber.Ctx) (seatID int64, userID int64, err error) {
	id, err := c.ParamsInt("seatId")
	if err != nil {
		return 0, 0, errors.New("Invalid seat id")
	}

	seatID = int64(id)
	var body struct {
		UserID int64 `json:"user_id"`
	}
	if err = c.BodyParser(&body); err != nil {
		return 0, 0, errors.New("Invalid request body")
	}
	return int64(seatID), body.UserID, nil
}
func errorToResponse(err error) (int, fiber.Map) {
	switch {
	case errors.Is(err, booking.ErrSeatNotFound):
		return fiber.StatusNotFound, fiber.Map{"error": "seat not found"}
	case errors.Is(err, booking.ErrSeatUnavailable):
		return fiber.StatusConflict, fiber.Map{"error": "seat unavailable"}
	case errors.Is(err, booking.ErrNoActiveHold):
		return fiber.StatusForbidden, fiber.Map{"error": "you do not hold this seat"}
	case errors.Is(err, booking.ErrHoldExpired):
		return fiber.StatusGone, fiber.Map{"error": "your hold has expired, please try again"}
	default:
		log.Printf("unexpected error: %v", err)
		return fiber.StatusInternalServerError, fiber.Map{"error": "internal error"}
	}
}

func (h *Handler) HoldSeat(c *fiber.Ctx) error {

	seatID, userID, err := ParseSeatRequest(c)
	log.Printf("parsed seatID=%d userID=%d", seatID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err = h.svc.HoldSeat(c.Context(), seatID, userID, 5*time.Minute); err != nil {
		status, body := errorToResponse(err)
		return c.Status(status).JSON(body)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "held"})
}

func (h *Handler) ConfirmBooking(c *fiber.Ctx) error {
	seatID, userID, err := ParseSeatRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.svc.BookSeat(c.Context(), seatID, userID); err != nil {
		status, body := errorToResponse(err)
		return c.Status(status).JSON(body)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "confirmed"})
}

func main() {

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	store := booking.NewStore(pool)
	svc := booking.NewService(store)
	h := &Handler{svc: svc}

	app := fiber.New()

	setUpRoutes(app, h)

	port := os.Getenv("APP_PORT")

	if port == "" {
		port = "3002"
	}

	if err := app.Listen(":" + port); err != nil {
		log.Fatal(err)
	}
}
