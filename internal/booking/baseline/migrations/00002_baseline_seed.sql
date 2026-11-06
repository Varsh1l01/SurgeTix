-- Deterministic synthetic event for the naive baseline (AMB-38): 500 seats,
-- section A, rows R01-R20 x 25 seats, 100000 paise each. Ids are fixed so k6
-- scripts and tests can address seats without a lookup:
--   event  5e1f0000-0000-4000-8000-000000000001
--   seat n 00000000-0000-4000-8000-<n zero-padded to 12 digits>, n = 1..500

-- +goose Up
INSERT INTO baseline.events (id, name)
VALUES ('5e1f0000-0000-4000-8000-000000000001', 'SurgeTix naive baseline synthetic event');

INSERT INTO baseline.seats (id, event_id, section, row_label, seat_number, price_paise)
SELECT ('00000000-0000-4000-8000-' || lpad(n::text, 12, '0'))::uuid,
       '5e1f0000-0000-4000-8000-000000000001'::uuid,
       'A',
       'R' || lpad(((n - 1) / 25 + 1)::text, 2, '0'),
       (n - 1) % 25 + 1,
       100000
FROM generate_series(1, 500) AS n;

-- +goose Down
DELETE FROM baseline.booking_seats;
DELETE FROM baseline.bookings;
DELETE FROM baseline.seats;
DELETE FROM baseline.events;
