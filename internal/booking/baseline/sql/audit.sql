-- Naive baseline audit: reconciles inventory with bookings. Run after every
-- benchmark run (CLAUDE.md, test plan). Used verbatim by the Go tests (embedded)
-- and by loadtest/baseline/run.sh (psql), so both read the same definitions.
--
--   seats_total            seats in the event
--   seats_booked           seats whose status is BOOKED
--   seats_available        seats whose status is AVAILABLE
--   bookings               successful booking requests (rows in bookings)
--   booked_seat_rows       seat assignments across all bookings
--   distinct_seats_sold    seats with at least one booking
--   double_booked_seats    seats with MORE THAN ONE booking  (SurgeTix: must be 0)
--   excess_seat_bookings   booked_seat_rows - distinct_seats_sold, i.e. seats sold beyond inventory
--   max_bookings_per_seat  worst single seat
--   status_mismatches      seats whose status disagrees with their bookings (expected 0)
SELECT
  (SELECT count(*) FROM baseline.seats)                                    AS seats_total,
  (SELECT count(*) FROM baseline.seats WHERE status = 'BOOKED')            AS seats_booked,
  (SELECT count(*) FROM baseline.seats WHERE status = 'AVAILABLE')         AS seats_available,
  (SELECT count(*) FROM baseline.bookings)                                 AS bookings,
  (SELECT count(*) FROM baseline.booking_seats)                            AS booked_seat_rows,
  (SELECT count(DISTINCT seat_id) FROM baseline.booking_seats)             AS distinct_seats_sold,
  (SELECT count(*) FROM (SELECT seat_id FROM baseline.booking_seats
                         GROUP BY seat_id HAVING count(*) > 1) d)          AS double_booked_seats,
  (SELECT count(*) - count(DISTINCT seat_id) FROM baseline.booking_seats)  AS excess_seat_bookings,
  (SELECT coalesce(max(c), 0) FROM (SELECT count(*) AS c FROM baseline.booking_seats
                                    GROUP BY seat_id) m)                   AS max_bookings_per_seat,
  (SELECT count(*) FROM baseline.seats s
    WHERE (s.status = 'BOOKED') <> EXISTS (SELECT 1 FROM baseline.booking_seats bs
                                            WHERE bs.seat_id = s.id))      AS status_mismatches;
