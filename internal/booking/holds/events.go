package holds

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/Varsh1l01/SurgeTix/internal/platform/outbox"
)

// Outbox `data` payloads, v1 (plan §9, AMB-47 ✅). Field names follow the
// plan's per-event lists. Times use outbox.OccurredAtLayout (RFC 3339 UTC,
// six fractional digits), the same format as the envelope.

type holdCreatedData struct {
	HoldID      string   `json:"hold_id"`
	EventID     string   `json:"event_id"`
	UserID      string   `json:"user_id"`
	Kind        string   `json:"kind"`
	ExpiresAt   string   `json:"expires_at"`
	AmountPaise int64    `json:"amount_paise"`
	Seats       []string `json:"seats"`
}

type seatStatus struct {
	SeatID string `json:"seat_id"`
	Status string `json:"status"`
}

type inventoryChangedData struct {
	EventID string       `json:"event_id"`
	Section string       `json:"section"`
	Seats   []seatStatus `json:"seats"`
}

type holdExpiredData struct {
	HoldID  string `json:"hold_id"`
	EventID string `json:"event_id"`
	Reason  string `json:"reason"`
}

type inventoryReturnedData struct {
	EventID string   `json:"event_id"`
	Section string   `json:"section"`
	Source  string   `json:"source"` // expired | released | reclaimed
	Seats   []string `json:"seats"`
}

// bySection groups seat ids by section, sections in ascending order and ids
// ascending within each, so events are written in a deterministic order.
func bySection(seats map[string]string) (sections []string, ids map[string][]string) {
	ids = map[string][]string{}
	for id, sec := range seats {
		ids[sec] = append(ids[sec], id)
	}
	for sec := range ids {
		sort.Strings(ids[sec])
		sections = append(sections, sec)
	}
	sort.Strings(sections)
	return sections, ids
}

func sectionKey(eventID, section string) string { return eventID + ":" + section }

// writeInventoryChanged writes one InventoryChanged per section (PDF p.24:
// a hold spanning two sections writes one per section).
func writeInventoryChanged(ctx context.Context, tx pgx.Tx, eventID, status string, seats map[string]string) error {
	sections, ids := bySection(seats)
	for _, sec := range sections {
		d := inventoryChangedData{EventID: eventID, Section: sec}
		for _, id := range ids[sec] {
			d.Seats = append(d.Seats, seatStatus{SeatID: id, Status: status})
		}
		if _, err := outbox.Insert(ctx, tx, outbox.Event{Type: outbox.InventoryChanged, Key: sectionKey(eventID, sec), Data: d}); err != nil {
			return err
		}
	}
	return nil
}

// writeInventoryReturned writes one InventoryReturned per section.
func writeInventoryReturned(ctx context.Context, tx pgx.Tx, eventID, source string, seats map[string]string) error {
	sections, ids := bySection(seats)
	for _, sec := range sections {
		d := inventoryReturnedData{EventID: eventID, Section: sec, Source: source, Seats: ids[sec]}
		if _, err := outbox.Insert(ctx, tx, outbox.Event{Type: outbox.InventoryReturned, Key: sectionKey(eventID, sec), Data: d}); err != nil {
			return err
		}
	}
	return nil
}
