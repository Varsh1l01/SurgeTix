package holds

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
)

// HoldScope is the idempotency scope of POST /v1/events/{eventID}/holds
// (plan §7.1). The event id itself is part of the hashed canonical request.
const HoldScope = "POST /v1/events/{eventID}/holds"

// abandonTimeout bounds the best-effort Abandon after a failure. It runs on
// a context detached from the request's, which may already be cancelled.
const abandonTimeout = 2 * time.Second

// Problem types of the service layer (plan §11.2). Like those in
// response.go, the URIs are provisional until the M2 review (AMB-12).
const (
	problemIdempotencyInProgress = "urn:surgetix:problem:idempotency-in-progress"
	problemIdempotencyKeyReused  = "urn:surgetix:problem:idempotency-key-reused"
	problemBadRequest            = "urn:surgetix:problem:bad-request"
	problemServiceUnavailable    = "urn:surgetix:problem:service-unavailable"
)

// ErrModeNotAvailable: best-available (M2) and GA (M3) holds are validated
// (T7) but not executed yet. Nothing is claimed or written for them, and no
// HTTP mapping exists until their milestone.
var ErrModeNotAvailable = errors.New("holds: this hold mode is not available yet")

// Response is the HTTP-level result of Hold: the status and JSON body to
// send, whether it is a replay of a stored response (Idempotent-Replayed:
// true, plan §11.2), and, for 503s, the underlying cause for logs and
// metrics (never sent to the client).
type Response struct {
	Status   int
	Body     json.RawMessage
	Replayed bool
	Cause    error
}

// Service is Booking's hold service (M1-T10): validation (T7), idempotency
// (T5) and the hold transaction (T8, T9) in one call.
type Service struct {
	pool  *pgxpool.Pool
	store *idempotency.Store

	// Test seams, nil/unset in production. abandon replaces the best-effort
	// Abandon (a test simulates a process that crashed before it could
	// abandon). beforeExecute runs between the claim and the transaction.
	abandon       func(ctx context.Context, c idempotency.Claim) (bool, error)
	beforeExecute func(ctx context.Context, c idempotency.Claim)
}

// NewService returns a Service using pool for every statement.
func NewService(pool *pgxpool.Pool) *Service {
	s := &Service{pool: pool, store: idempotency.New(pool)}
	s.abandon = s.store.Abandon
	return s
}

// Hold handles POST /v1/events/{eventID}/holds for user with the client's
// Idempotency-Key (plan §4, §7, §11.2):
//
//	user or key missing/malformed      400 bad-request             (nothing claimed)
//	invalid body, tier mode before M4  422 invalid-request         (not claimed: §4.2 validates before the claim)
//	key used for a different request   422 idempotency-key-reused  (the original stays)
//	key in flight elsewhere            409 idempotency-in-progress (retry later)
//	key completed                      the stored 201 / 409 / 422, Replayed
//	executed now                       201 hold, 409 seats-unavailable, 422 unknown seats (all stored)
//	lost claim, lock timeout, deadlock,
//	cancellation, database failure     503 (nothing stored; claim abandoned when still ours)
//
// A retry with the same key and request therefore returns the original
// committed result and never creates a second hold.
func (s *Service) Hold(ctx context.Context, userID, key, eventID string, body []byte) (Response, error) {
	if !uuidRE.MatchString(userID) {
		return problem(http.StatusBadRequest, problemBadRequest, "Bad request", "missing or invalid caller identity"), nil
	}
	if key == "" {
		return problem(http.StatusBadRequest, problemBadRequest, "Bad request", "missing Idempotency-Key"), nil
	}
	req, err := ParseRequest(eventID, body)
	if err != nil {
		return problem(http.StatusUnprocessableEntity, problemInvalidRequest, "Invalid request", err.Error()), nil
	}
	if req.Mode != ModeSeats {
		return Response{}, ErrModeNotAvailable
	}
	canon, err := req.Canonical()
	if err != nil {
		return Response{}, err
	}
	hash, err := idempotency.RequestHash(HoldScope, canon)
	if err != nil {
		return Response{}, err
	}

	res, err := s.store.Claim(ctx, idempotency.Key{Scope: HoldScope, UserID: userID, Key: key}, hash)
	if err != nil {
		return unavailable(err), nil // nothing claimed, nothing to abandon
	}
	switch res.Outcome {
	case idempotency.Replay:
		return Response{Status: res.Response.Code, Body: res.Response.Body, Replayed: true}, nil
	case idempotency.InProgress:
		return problem(http.StatusConflict, problemIdempotencyInProgress, "Request in progress",
			"another request with this Idempotency-Key is in flight; retry later"), nil
	case idempotency.Mismatch:
		return problem(http.StatusUnprocessableEntity, problemIdempotencyKeyReused, "Idempotency-Key reused",
			"this Idempotency-Key was used for a different request"), nil
	case idempotency.Proceed:
	default:
		return Response{}, errors.New("holds: unknown claim outcome " + res.Outcome.String())
	}

	if s.beforeExecute != nil {
		s.beforeExecute(ctx, res.Claim)
	}
	out, err := HoldSeats(ctx, s.pool, res.Claim, req)
	if err == nil {
		return Response{Status: out.Response.Code, Body: out.Response.Body}, nil
	}
	if !errors.Is(err, idempotency.ErrNotOwner) {
		// Non-deterministic failure: nothing was stored. Give the claim up
		// at once so the retry need not wait out the 30 s lease (plan
		// §7.4). Best effort and fenced: if it fails, the lease frees it.
		actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), abandonTimeout)
		_, _ = s.abandon(actx, res.Claim)
		cancel()
	}
	// ErrNotOwner: another request took the key over (or completed it);
	// it is not ours to abandon. The client's retry replays or waits.
	return unavailable(err), nil
}

type problemBody struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func problem(status int, typ, title, detail string) Response {
	b, _ := json.Marshal(problemBody{Type: typ, Title: title, Status: status, Detail: detail})
	return Response{Status: status, Body: b}
}

func unavailable(cause error) Response {
	r := problem(http.StatusServiceUnavailable, problemServiceUnavailable, "Service unavailable",
		"the request did not complete; retry with the same Idempotency-Key")
	r.Cause = cause
	return r
}
