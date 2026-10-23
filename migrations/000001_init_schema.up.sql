CREATE TABLE movies (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    description text,
    duration_minutes integer NOT NULL
);

CREATE TABLE screenings (
    id bigserial PRIMARY KEY,
    movie_id bigint NOT NULL REFERENCES movies(id),
    starts_at timestamptz NOT NULL
);

CREATE TABLE seats (
    id bigserial PRIMARY KEY,
    screening_id bigint NOT NULL REFERENCES screenings(id),
    label text NOT NULL,
    status text NOT NULL DEFAULT 'available'
        CHECK (status IN ('available', 'booked')),
    UNIQUE (screening_id, label)
);

CREATE TABLE bookings (
    id bigserial PRIMARY KEY,
    screening_id bigint NOT NULL REFERENCES screenings(id),
    seat_id bigint NOT NULL REFERENCES seats(id),
    user_id bigint NOT NULL,
    status text NOT NULL DEFAULT 'confirmed'
        CHECK (status IN ('confirmed', 'cancelled')),
    created_at timestamptz NOT NULL DEFAULT now()
);