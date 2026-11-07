-- Booking audit (Phase 3 plan §13.3, ADR-036). READ-ONLY: SELECTs only.
-- Audit() runs this in a READ ONLY, REPEATABLE READ transaction, so every check
-- sees one consistent snapshot and one fixed now(), and nothing can be written.
--
-- One row per finding: (check, event_id, subject, detail).
--   check    stable name of the violated invariant (see audit.go for sources)
--   event_id the event the finding belongs to
--   subject  the affected record(s): seat id, "section/bucket", hold id, ticket id
--   detail   the facts that violate it
--
-- Who CLAIMS a seat:
--   * an ACTIVE/PAYMENT_PENDING hold not yet past expires_at, through hold_seats;
--   * a CONVERTED hold only while its booking has a valid (non-cancelled)
--     ticket for the seat. Cancelling a ticket frees the seat for resale
--     (PDF p.32), while the old hold keeps its hold_seats rows as history.
-- An ACTIVE/PAYMENT_PENDING hold past expires_at is a valid state awaiting the
-- expiry worker or inline reclaim (PDF p.28, p.30). Its seats may already
-- belong to a newer hold (partial reclaim, plan §8.3). It claims nothing.

-- C1 double_booked_seat: a seat claimed by two or more holds (PDF p.34 metric).
SELECT 'double_booked_seat' AS check_name, c.event_id::text AS event_id, c.seat_id AS subject,
       'claimed by ' || count(*) || ' holds: ' || string_agg(c.hold_id::text || ' (' || c.status || ')', ', ' ORDER BY c.hold_id) AS detail
  FROM (SELECT hs.event_id, hs.seat_id, h.hold_id, h.status
          FROM hold_seats hs JOIN holds h ON h.hold_id = hs.hold_id
         WHERE h.status IN ('ACTIVE', 'PAYMENT_PENDING') AND h.expires_at > now()
        UNION ALL
        SELECT hs.event_id, hs.seat_id, h.hold_id, h.status
          FROM hold_seats hs
          JOIN holds h    ON h.hold_id = hs.hold_id AND h.status = 'CONVERTED'
          JOIN bookings b ON b.hold_id = h.hold_id
         WHERE EXISTS (SELECT 1 FROM tickets t
                        WHERE t.booking_id = b.booking_id AND t.event_id = hs.event_id
                          AND t.seat_id = hs.seat_id AND t.status <> 'CANCELLED')) c
 GROUP BY c.event_id, c.seat_id
HAVING count(*) > 1

UNION ALL
-- C2a held_seat_without_hold: a HELD seat whose hold_id matches no hold.
SELECT 'held_seat_without_hold', s.event_id::text, s.seat_id,
       'seat HELD by hold ' || s.hold_id || ', which does not exist'
  FROM seats s
 WHERE s.status = 'HELD'
   AND NOT EXISTS (SELECT 1 FROM holds h WHERE h.hold_id = s.hold_id)

UNION ALL
-- C2b held_seat_mirror_mismatch: seats.hold_expires_at must equal the canonical
-- holds.expires_at (PDF p.21 invariant).
SELECT 'held_seat_mirror_mismatch', s.event_id::text, s.seat_id,
       'seat mirror ' || s.hold_expires_at || ' <> hold ' || h.hold_id || ' expires_at ' || h.expires_at
  FROM seats s
  JOIN holds h ON h.hold_id = s.hold_id
 WHERE s.status = 'HELD'
   AND s.hold_expires_at IS DISTINCT FROM h.expires_at

UNION ALL
-- C3 orphan_held_seat: a seat still HELD by a hold that is no longer
-- ACTIVE/PAYMENT_PENDING (plan §13.3 "no orphans", AMB-50).
SELECT 'orphan_held_seat', s.event_id::text, s.seat_id,
       'seat HELD by hold ' || h.hold_id || ' whose status is ' || h.status
  FROM seats s
  JOIN holds h ON h.hold_id = s.hold_id
 WHERE s.status = 'HELD'
   AND h.status NOT IN ('ACTIVE', 'PAYMENT_PENDING')

UNION ALL
-- C4 held_seat_not_in_hold: the hold owning a HELD seat must list it in
-- hold_seats, the normalised seats of that hold (PDF p.18).
SELECT 'held_seat_not_in_hold', s.event_id::text, s.seat_id,
       'seat HELD by hold ' || s.hold_id || ' but not among that hold''s hold_seats'
  FROM seats s
 WHERE s.status = 'HELD'
   AND EXISTS (SELECT 1 FROM holds h WHERE h.hold_id = s.hold_id)
   AND NOT EXISTS (SELECT 1 FROM hold_seats hs
                    WHERE hs.hold_id = s.hold_id AND hs.event_id = s.event_id AND hs.seat_id = s.seat_id)

UNION ALL
-- C5 oversold_ga: unreleased allocations exceed a section's capacity (PDF p.34 metric).
SELECT 'oversold_ga', b.event_id::text, b.section,
       'unreleased allocations ' || coalesce(a.taken, 0) || ' > capacity ' || b.capacity
  FROM (SELECT event_id, section, sum(capacity) AS capacity FROM ga_buckets GROUP BY event_id, section) b
  LEFT JOIN (SELECT event_id, section, sum(quantity) AS taken FROM hold_ga_allocations
              WHERE released_at IS NULL GROUP BY event_id, section) a
         ON a.event_id = b.event_id AND a.section = b.section
 WHERE coalesce(a.taken, 0) > b.capacity

UNION ALL
-- C6 ga_stock_mismatch: per bucket, capacity - remaining must equal the
-- unreleased allocations (plan §13.3 conservation). A surplus (remaining too
-- high) means stock restored twice or never taken. A deficit means stock taken
-- without a recorded allocation.
SELECT 'ga_stock_mismatch', b.event_id::text, b.section || '/' || b.bucket,
       'capacity ' || b.capacity || ' - remaining ' || b.remaining || ' = ' || (b.capacity - b.remaining)
         || ' but unreleased allocations = ' || coalesce(a.taken, 0)
         || CASE WHEN b.capacity - b.remaining < coalesce(a.taken, 0) THEN ' (surplus: stock restored twice or never taken)'
                 ELSE ' (deficit: stock taken without an allocation)' END
  FROM ga_buckets b
  LEFT JOIN (SELECT event_id, section, bucket, sum(quantity) AS taken FROM hold_ga_allocations
              WHERE released_at IS NULL GROUP BY event_id, section, bucket) a
         ON a.event_id = b.event_id AND a.section = b.section AND a.bucket = b.bucket
 WHERE b.capacity - b.remaining <> coalesce(a.taken, 0)

UNION ALL
-- C7 orphan_ga_allocation: an EXPIRED/RELEASED hold must have restored all its
-- GA stock (plan §8.1: such a hold has zero unreleased allocations).
SELECT 'orphan_ga_allocation', a.event_id::text, a.hold_id || ' ' || a.section || '/' || a.bucket,
       a.quantity || ' unit(s) still allocated to hold ' || a.hold_id || ' whose status is ' || h.status
  FROM hold_ga_allocations a
  JOIN holds h ON h.hold_id = a.hold_id
 WHERE a.released_at IS NULL
   AND h.status IN ('EXPIRED', 'RELEASED')

UNION ALL
-- C8 booked_seat_ticket_count: every BOOKED seat has exactly one valid
-- (non-cancelled) ticket (PDF FR-22, Booking's part of the nightly audit).
SELECT 'booked_seat_ticket_count', s.event_id::text, s.seat_id,
       'BOOKED seat has ' || count(t.ticket_id) || ' valid tickets (want 1)'
  FROM seats s
  LEFT JOIN tickets t ON t.event_id = s.event_id AND t.seat_id = s.seat_id AND t.status <> 'CANCELLED'
 WHERE s.status = 'BOOKED'
 GROUP BY s.event_id, s.seat_id
HAVING count(t.ticket_id) <> 1

UNION ALL
-- C9 live_ticket_on_unbooked_seat: a valid ticket's seat must be BOOKED.
-- Tickets are issued with the booking and cancelled with the seat's return in
-- the same transaction (PDF p.29-30).
SELECT 'live_ticket_on_unbooked_seat', t.event_id::text, t.ticket_id::text,
       'ticket ' || t.status || ' for seat ' || t.seat_id || ' whose status is ' || s.status
  FROM tickets t
  JOIN seats s ON s.event_id = t.event_id AND s.seat_id = t.seat_id
 WHERE t.seat_id IS NOT NULL
   AND t.status <> 'CANCELLED'
   AND s.status <> 'BOOKED'

ORDER BY 1, 2, 3
