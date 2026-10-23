INSERT INTO movies (name, description, duration_minutes)
VALUES ('Test Movie', 'Seed movie for concurrency experiments', 120);

-- Fresh database, so the movie above has id = 1
INSERT INTO screenings (movie_id, starts_at)
VALUES (1, now() + interval '1 day');

-- 5 rows (A-E) x 10 seats each = 50 seats for screening 1
INSERT INTO seats (screening_id, label)
SELECT 1, chr(64 + r) || n::text
FROM generate_series(1, 5)  AS r,
     generate_series(1, 10) AS n;