-- Reset the naive baseline to its seeded state (all 500 seats AVAILABLE, no
-- bookings). Run before every benchmark run and by the tests. Touches only the
-- "baseline" schema.
TRUNCATE baseline.booking_seats, baseline.bookings;
UPDATE baseline.seats SET status = 'AVAILABLE', booked_at = NULL;
