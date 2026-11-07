// Package outbox writes events to the transactional outbox (ADR-009, PDF
// p.21, p.24, p.28; ADR-036 / AMB-03, AMB-47). It is shared infrastructure
// and knows nothing about Booking's domain types: callers pass an event
// type, its partition key and a JSON-object payload (`data`).
//
// Insert runs inside the CALLER's PostgreSQL transaction, so a state change
// and its events commit together or not at all (invariant 8). It never
// commits, rolls back or talks to Kafka. Rows are left PENDING with the
// schema defaults; publishing them is the phase 4 relay's job (TASKS P4-5).
//
// Envelope v1 (JSON, UTF-8, stored in outbox.payload), fields in this order:
//
//	event_id        = outbox.id, a UUID from gen_random_uuid()
//	event_type      one of the registered PDF event names
//	schema_version  1
//	occurred_at     the transaction's now(), RFC 3339 UTC, 6 fractional digits
//	producer        "booking"
//	aggregate       {"type": "hold"|"section", "id": <outbox.key>}
//	data            the caller's JSON object
//
// The topic, aggregate type, producer and schema version are fixed per event
// type in the registry below; a caller cannot override them. SoldOut is not
// registered until its emission condition is decided (AMB-52).
//
// Not in this package (yet): the aggregate's version that p.24 asks every
// event to carry (AMB-54, open), the relay, topic creation and retries.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Event types (PDF p.24, AMB-47). Names are stable.
const (
	HoldCreated       = "HoldCreated"
	HoldReleased      = "HoldReleased"
	HoldExpired       = "HoldExpired"
	InventoryChanged  = "InventoryChanged"
	InventoryReturned = "InventoryReturned"
)

// Topics (PDF p.24 table; AMB-03 ✅).
const (
	TopicHolds     = "booking.holds"
	TopicInventory = "booking.inventory"
)

// SchemaVersion is the envelope version written by this package.
const SchemaVersion = 1

// OccurredAtLayout formats occurred_at: RFC 3339, UTC, exactly six
// fractional digits (PostgreSQL's timestamp resolution).
const OccurredAtLayout = "2006-01-02T15:04:05.000000Z"

// ErrInvalidEvent is returned (wrapped) for an event rejected before any SQL
// runs. A rejected call writes nothing and leaves the transaction usable.
var ErrInvalidEvent = errors.New("outbox: invalid event")

// Event is one event to write. Key is the Kafka partition key and the
// aggregate id: a hold UUID for hold events, "<event uuid>:<section>" for
// inventory events. Data must marshal to a JSON object.
type Event struct {
	Type string
	Key  string
	Data any
}

type keyForm int

const (
	keyHold    keyForm = iota // "<hold uuid>"
	keySection                // "<event uuid>:<section>"
)

type spec struct {
	topic         string
	aggregateType string
	producer      string
	key           keyForm
}

var registry = map[string]spec{
	HoldCreated:       {TopicHolds, "hold", "booking", keyHold},
	HoldReleased:      {TopicHolds, "hold", "booking", keyHold},
	HoldExpired:       {TopicHolds, "hold", "booking", keyHold},
	InventoryChanged:  {TopicInventory, "section", "booking", keySection},
	InventoryReturned: {TopicInventory, "section", "booking", keySection},
}

// Lower-case canonical form only (PostgreSQL's uuid text output). Two
// spellings of one UUID would otherwise land on different partitions.
var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type aggregate struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	SchemaVersion int             `json:"schema_version"`
	OccurredAt    string          `json:"occurred_at"`
	Producer      string          `json:"producer"`
	Aggregate     aggregate       `json:"aggregate"`
	Data          json.RawMessage `json:"data"`
}

// Insert writes e to the outbox inside tx and returns its event_id
// (= outbox.id). It runs two parameterised statements in tx and takes no
// lock other than on its own new row, which no other transaction can see.
func Insert(ctx context.Context, tx pgx.Tx, e Event) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("%w: nil transaction", ErrInvalidEvent)
	}
	s, data, err := validate(e)
	if err != nil {
		return "", err
	}

	var (
		id         string
		occurredAt time.Time
	)
	// now() is the transaction start time, the same for every event in tx.
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text, now()`).Scan(&id, &occurredAt); err != nil {
		return "", fmt.Errorf("outbox: allocate id: %w", err)
	}
	payload, err := encode(id, occurredAt, e.Type, s, e.Key, data)
	if err != nil {
		return "", err
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO outbox (id, topic, key, payload) VALUES ($1::uuid, $2, $3, $4)`,
		id, s.topic, e.Key, payload)
	if err != nil {
		return "", fmt.Errorf("outbox: insert: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return "", fmt.Errorf("outbox: insert affected %d rows, want 1", tag.RowsAffected())
	}
	return id, nil
}

// validate checks e against the registry and returns its spec and its data
// as compact JSON. It runs no SQL.
func validate(e Event) (spec, json.RawMessage, error) {
	s, ok := registry[e.Type]
	if !ok {
		return spec{}, nil, fmt.Errorf("%w: unknown or unsupported event type %q", ErrInvalidEvent, e.Type)
	}
	if err := checkKey(s.key, e.Key); err != nil {
		return spec{}, nil, err
	}
	if e.Data == nil {
		return spec{}, nil, fmt.Errorf("%w: data is nil", ErrInvalidEvent)
	}
	data, err := json.Marshal(e.Data)
	if err != nil {
		return spec{}, nil, fmt.Errorf("%w: data: %v", ErrInvalidEvent, err)
	}
	if len(data) == 0 || data[0] != '{' {
		return spec{}, nil, fmt.Errorf("%w: data must be a JSON object", ErrInvalidEvent)
	}
	return s, data, nil
}

func checkKey(form keyForm, key string) error {
	switch form {
	case keyHold:
		if !uuidRE.MatchString(key) {
			return fmt.Errorf("%w: key %q is not a lower-case hold UUID", ErrInvalidEvent, key)
		}
	case keySection:
		eventID, section, ok := strings.Cut(key, ":")
		if !ok || !uuidRE.MatchString(eventID) {
			return fmt.Errorf("%w: key %q is not <lower-case event UUID>:<section>", ErrInvalidEvent, key)
		}
		if section == "" || !utf8.ValidString(section) {
			return fmt.Errorf("%w: key %q has an empty or invalid section", ErrInvalidEvent, key)
		}
	}
	return nil
}

// encode builds the envelope v1 bytes. Field order is fixed by the struct.
func encode(id string, occurredAt time.Time, eventType string, s spec, key string, data json.RawMessage) ([]byte, error) {
	b, err := json.Marshal(envelope{
		EventID:       id,
		EventType:     eventType,
		SchemaVersion: SchemaVersion,
		OccurredAt:    occurredAt.UTC().Format(OccurredAtLayout),
		Producer:      s.producer,
		Aggregate:     aggregate{Type: s.aggregateType, ID: key},
		Data:          data,
	})
	if err != nil {
		return nil, fmt.Errorf("outbox: encode: %w", err)
	}
	return b, nil
}
