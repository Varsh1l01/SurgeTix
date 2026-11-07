package holds

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Varsh1l01/SurgeTix/internal/platform/idempotency"
)

// Response bodies stored with the idempotency record and returned to the
// caller (plan §11.2). Problem bodies follow RFC 9457. The problem type URIs
// and extension member names are the plan's provisional values, confirmed
// at the M2 review (AMB-12).

const (
	problemSeatsUnavailable = "urn:surgetix:problem:seats-unavailable"
	problemInvalidRequest   = "urn:surgetix:problem:invalid-request"
)

// HeldSeat is one seat of a 201 response (plan §11.2 `seats`).
type HeldSeat struct {
	SeatID     string `json:"seat_id"`
	Section    string `json:"section"`
	Row        string `json:"row"`
	Number     int32  `json:"number"`
	PricePaise int64  `json:"price_paise"`
}

// Created is the 201 body: {hold_id, seats, expires_at, amount} (PDF p.29).
type Created struct {
	HoldID    string     `json:"hold_id"`
	Seats     []HeldSeat `json:"seats"`
	ExpiresAt string     `json:"expires_at"` // RFC 3339 UTC, six fractional digits
	Amount    int64      `json:"amount"`     // integer paise (invariant 14)
}

type seatsUnavailableProblem struct {
	Type               string   `json:"type"`
	Title              string   `json:"title"`
	Status             int      `json:"status"`
	Detail             string   `json:"detail"`
	UnavailableSeatIDs []string `json:"unavailable_seat_ids"`
}

type unknownSeatsProblem struct {
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Status         int      `json:"status"`
	Detail         string   `json:"detail"`
	UnknownSeatIDs []string `json:"unknown_seat_ids"`
}

func jsonResponse(code int, v any) (idempotency.Response, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return idempotency.Response{}, fmt.Errorf("holds: encode %d response: %w", code, err)
	}
	return idempotency.Response{Code: code, Body: b}, nil
}

func createdResponse(c Created) (idempotency.Response, error) {
	return jsonResponse(http.StatusCreated, c)
}

func unavailableResponse(ids []string) (idempotency.Response, error) {
	return jsonResponse(http.StatusConflict, seatsUnavailableProblem{
		Type:               problemSeatsUnavailable,
		Title:              "Seats unavailable",
		Status:             http.StatusConflict,
		Detail:             "one or more requested seats are not available",
		UnavailableSeatIDs: ids,
	})
}

func unknownSeatsResponse(ids []string) (idempotency.Response, error) {
	return jsonResponse(http.StatusUnprocessableEntity, unknownSeatsProblem{
		Type:           problemInvalidRequest,
		Title:          "Invalid request",
		Status:         http.StatusUnprocessableEntity,
		Detail:         "one or more requested seats do not exist for this event",
		UnknownSeatIDs: ids,
	})
}
