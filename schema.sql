CREATE TABLE movies (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT,
    duration_minutes INT  NOT NULL
);

CREATE TABLE screenings (
    id        BIGSERIAL PRIMARY KEY,
    movie_id  BIGINT      NOT NULL REFERENCES movies(id),
    starts_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE seats (
    id           BIGSERIAL PRIMARY KEY,
    screening_id BIGINT NOT NULL REFERENCES screenings(id),
    label        TEXT   NOT NULL,                       -- e.g. 'A1', 'C7'
    status       TEXT   NOT NULL DEFAULT 'available'
                 CHECK (status IN ('available', 'booked')),
    UNIQUE (screening_id, label)                        -- no two seats with the same label in one screening
);

CREATE TABLE bookings (
    id           BIGSERIAL PRIMARY KEY,
    screening_id BIGINT      NOT NULL REFERENCES screenings(id),
    seat_id      BIGINT      NOT NULL REFERENCES seats(id),
    user_id      BIGINT      NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'confirmed'
                 CHECK (status IN ('confirmed', 'cancelled')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
    -- NOTE: there is deliberately NO unique constraint on seat_id yet.
    -- Stage 1 needs to show the double-booking bug. We add the safety net in Stage 2.
);

ALTER TABLE seats
    ADD COLUMN held_by BIGINT
    Add COLUMN held_until TIMESTAMPTZ