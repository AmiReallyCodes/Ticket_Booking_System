# Ticket Booking System

A concurrency-safe seat-booking backend built in Go, using Fiber (HTTP) and PostgreSQL (via pgx). Built as a learning project focused specifically on correctly handling race conditions under concurrent load — not just making the happy path work.

## What it does

- Lets users **hold** a seat temporarily (with an expiry/TTL) while they decide to book
- Lets users **confirm** a held seat into a real booking
- Guarantees — even under heavy concurrent load — that a seat can never be double-booked, and that only the user who holds a seat can confirm it

## Why this project exists

Most CRUD tutorials skip the hard part: what happens when two requests hit the same row at the same time? This project is built specifically to answer that, using PostgreSQL's row-level locking instead of application-level mutexes or external coordination services.

## Core design

Every state-changing operation uses a single atomic `UPDATE ... WHERE <condition>` (or a `SELECT ... FOR UPDATE` inside a transaction when more than one outcome needs to be distinguished). Postgres takes a row lock the instant the statement runs, so concurrent requests serialize naturally — the database enforces correctness, not the application code.

```sql
-- Booking a seat: only succeeds if it's still available
UPDATE seats SET status = 'booked' WHERE id = $1 AND status = 'available';

-- Holding a seat: only succeeds if it's available and not already validly held
UPDATE seats
SET held_by = $1, held_until = now() + $2::interval
WHERE id = $3
  AND status = 'available'
  AND (held_until IS NULL OR held_until < now());
```

## Load test results

Measured directly, not estimated — see `internal/booking/scenarios_test.go` to reproduce.

| Test | Result |
|---|---|
| 200 concurrent holds, same seat (direct store call) | Exactly 1 succeeded, 0 double-bookings |
| 500 concurrent holds, 50 seats (direct store call) | Exactly 50 succeeded (one per seat), ~2,678 req/sec |
| 200 concurrent holds, same seat (through live HTTP API) | 1 success, 199 correctly rejected (409), 0 double-bookings, ~1,161 req/sec, p99 ~112ms |
| Confirm without an active hold | Correctly rejected (403) |
| Confirm after hold expiry | Correctly rejected (410), seat released to next user |

## Tech stack

- **Go** — application logic
- **Fiber** — HTTP routing
- **PostgreSQL** (via `pgx/v5`) — storage and concurrency control
- **golang-migrate** — schema migrations
- **hey** — HTTP load testing

## API

| Method | Route | Body | Description |
|---|---|---|---|
| POST | `/api/seats/:seatId/hold` | `{"user_id": 1}` | Temporarily hold a seat |
| POST | `/api/seats/:seatId/confirm` | `{"user_id": 1}` | Confirm a held seat into a real booking |

**Error responses:**

| Status | Meaning |
|---|---|
| 404 | Seat doesn't exist |
| 409 | Seat already booked |
| 403 | This user doesn't hold the seat |
| 410 | Hold expired |

## Project structure

```
api/                    — Fiber server, HTTP handlers, routing
internal/booking/       — domain models, Postgres store, business logic (service layer)
migrations/             — versioned schema changes (golang-migrate)
```

Layered as: **Handler (HTTP)** → **Service (business rules)** → **Store (SQL)**. Each layer depends only on the layer below's interface, not its concrete implementation — lets the business logic be tested with a fake in-memory store instead of a real database.

## Setup

**Requirements:** Go, Docker (for Postgres), `golang-migrate` CLI

1. Start Postgres:
   ```
   docker run --name booking-postgres -e POSTGRES_USER=booking -e POSTGRES_PASSWORD=booking -e POSTGRES_DB=booking -p 5433:5432 -d postgres
   ```

2. Run migrations:
   ```
   migrate -database "postgres://booking:booking@localhost:5433/booking?sslmode=disable" -path migrations up
   ```

3. Set the connection string and run the server:
   ```
   $env:DATABASE_URL = "postgres://booking:booking@localhost:5433/booking?sslmode=disable"
   go run ./api
   ```

4. Run the tests:
   ```
   go test -v -count=1 ./internal/booking/...
   ```

## What I learned building this

- How to use Postgres row locks (`UPDATE ... WHERE`, `SELECT ... FOR UPDATE`) to solve race conditions without external locking infrastructure
- Why dependency inversion (`Service` depending on an interface, not a concrete `*Store`) makes business logic testable without a database
- Debugging a real channel-based deadlock in a Go worker pool, and why `context` cancellation doesn't protect a blocking call outside the `select` that checks it
- Writing and running schema migrations with `golang-migrate` instead of hand-run `ALTER TABLE` statements