// Package idempotency implements Idempotency-Key handling for every
// state-changing operation (FR-12, PDF p.28; ADR-036 / AMB-48). It is shared
// infrastructure: Booking uses it from phase 3, Checkout later.
//
// Lifecycle of a key (scope, user_id, key):
//
//  1. Claim, in its own short transaction holding no other lock, returns one
//     of four outcomes:
//     - Proceed: the caller owns the key at a generation (its fencing token);
//     - Replay: a stored response exists for this request;
//     - InProgress: another request with this key holds a live claim;
//     - Mismatch: the key was used for a different request.
//  2. The caller runs its business transaction. Its FIRST statement is
//     Fence(tx, claim), which locks the claim row (lock rank 0) and proves
//     ownership. Its LAST statement before COMMIT is Complete(tx, claim,
//     response). The effect and the stored response therefore commit
//     together, or not at all.
//  3. On a non-deterministic failure (timeout, 5xx), the caller may Abandon
//     the claim (invalidate it, not delete it) so a retry can take over at
//     once. Otherwise the 30-second lease lets a retry take over.
//
// Fencing: every claim, re-claim, takeover and abandon changes the token
// (claim_generation, claimed_at). Fence, Complete and Abandon match the
// caller's token, so an old claimant can never complete, overwrite or
// invalidate a newer claim. A claimant
// inside its business transaction holds the row lock from Fence. A takeover
// attempted meanwhile waits for it, then sees COMPLETED and replays.
//
// Lease and retention are the approved fixed values (30 s, 24 h). All times
// come from the database clock. All SQL is parameterised.
package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Approved parameters (AMB-48). Not configurable.
const (
	Lease     = 30 * time.Second
	Retention = 24 * time.Hour
)

// Interval literals for SQL, kept next to the constants they mirror.
const (
	leaseSQL     = "30 seconds"
	retentionSQL = "24 hours"
)

// Key identifies one idempotency record.
type Key struct {
	Scope  string // the operation, e.g. "POST /v1/events/{eventID}/holds"
	UserID string // UUID of the caller
	Key    string // the client's Idempotency-Key
}

// Claim is the ownership token of a caller that may execute the operation.
// The token is (Generation, ClaimedAt), both stored on the row and both
// changed by every claim, takeover and abandon. Generation alone is not
// enough: if a row is ever deleted (retention cleanup) and the key reused, a
// fresh row starts at generation 1 again, but it can never carry the old
// claimed_at.
type Claim struct {
	Key
	Generation  int64
	ClaimedAt   time.Time
	RequestHash []byte
}

// Response is a stored deterministic result (e.g. 201, or a business 409).
type Response struct {
	Code int
	Body json.RawMessage // must be valid JSON (stored as JSONB)
}

// Outcome of Claim.
type Outcome int

const (
	Proceed    Outcome = iota + 1 // execute the operation, owning Result.Claim
	Replay                        // return Result.Response, do not execute
	InProgress                    // another request owns a live claim: retry later
	Mismatch                      // the key was used for a different request
)

func (o Outcome) String() string {
	return map[Outcome]string{Proceed: "proceed", Replay: "replay", InProgress: "in_progress", Mismatch: "mismatch"}[o]
}

// Result of Claim. Claim is set for Proceed, Response for Replay.
type Result struct {
	Outcome  Outcome
	Claim    Claim
	Response Response
}

// ErrNotOwner means the caller's claim is no longer current: it was taken
// over, completed, abandoned or expired. The caller must roll back without
// any effect.
var ErrNotOwner = errors.New("idempotency: claim is not owned by this caller (taken over, completed or gone)")

// Store claims and maintains idempotency records in the service's own database.
type Store struct{ pool *pgxpool.Pool }

// New returns a Store over the service's pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (k Key) validate() (pgtype.UUID, error) {
	var u pgtype.UUID
	if k.Scope == "" || k.Key == "" {
		return u, errors.New("idempotency: scope and key are required")
	}
	if err := u.Scan(k.UserID); err != nil {
		return u, fmt.Errorf("idempotency: user id %q: %w", k.UserID, err)
	}
	return u, nil
}

// maxClaimAttempts bounds re-reads after losing a race (the row changed
// between our read and our conditional write). Each re-read observes a newer
// state, so a few attempts always suffice in practice. Running out is
// reported as InProgress, the safe answer (the client retries).
const maxClaimAttempts = 5

// Claim acquires the key for requestHash, or reports why not. It runs in its
// own transaction(s) and holds no other lock, so it may wait on the key's row
// lock (rank 0) without breaking the lock order (plan §10).
func (s *Store) Claim(ctx context.Context, k Key, requestHash []byte) (Result, error) {
	user, err := k.validate()
	if err != nil {
		return Result{}, err
	}
	if len(requestHash) == 0 {
		return Result{}, errors.New("idempotency: request hash is required")
	}
	for range maxClaimAttempts {
		// 1. Fresh key: insert as IN_PROGRESS, generation 1.
		var gen int64
		var at time.Time
		err := s.pool.QueryRow(ctx, `INSERT INTO idempotency_keys
				(scope, user_id, key, request_hash, status, expires_at)
			VALUES ($1, $2, $3, $4, 'IN_PROGRESS', now() + interval '`+retentionSQL+`')
			ON CONFLICT (scope, user_id, key) DO NOTHING
			RETURNING claim_generation, claimed_at`, k.Scope, user, k.Key, requestHash).Scan(&gen, &at)
		if err == nil {
			return Result{Outcome: Proceed, Claim: Claim{Key: k, Generation: gen, ClaimedAt: at, RequestHash: requestHash}}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Result{}, err
		}

		// 2. The key exists: read it and decide.
		var (
			hash           []byte
			status         string
			code           *int32
			body           []byte
			expired, stale bool
		)
		err = s.pool.QueryRow(ctx, `SELECT request_hash, status, response_code, response_body,
				expires_at <= now(), claimed_at <= now() - interval '`+leaseSQL+`'
			FROM idempotency_keys WHERE scope = $1 AND user_id = $2 AND key = $3`,
			k.Scope, user, k.Key).Scan(&hash, &status, &code, &body, &expired, &stale)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // abandoned or cleaned up meanwhile: insert again
		}
		if err != nil {
			return Result{}, err
		}

		switch {
		case expired:
			// Past retention: the old record no longer exists logically.
			// Replace it in place, as a new request (fenced by generation).
			err = s.pool.QueryRow(ctx, `UPDATE idempotency_keys
				SET request_hash = $4, status = 'IN_PROGRESS', response_code = NULL, response_body = NULL,
				    claim_generation = claim_generation + 1, claimed_at = now(), created_at = now(),
				    expires_at = now() + interval '`+retentionSQL+`'
				WHERE scope = $1 AND user_id = $2 AND key = $3 AND expires_at <= now()
				RETURNING claim_generation, claimed_at`, k.Scope, user, k.Key, requestHash).Scan(&gen, &at)
		case string(hash) != string(requestHash):
			return Result{Outcome: Mismatch}, nil
		case status == "COMPLETED":
			r := Response{Body: body}
			if code != nil {
				r.Code = int(*code)
			}
			return Result{Outcome: Replay, Response: r}, nil
		case !stale:
			return Result{Outcome: InProgress}, nil
		default:
			// Stale claim (the owner crashed or stalled past the lease):
			// take over with a new generation. If the owner is inside its
			// business transaction it holds the row lock, so this waits,
			// then matches nothing (now COMPLETED) and we re-read.
			err = s.pool.QueryRow(ctx, `UPDATE idempotency_keys
				SET claim_generation = claim_generation + 1, claimed_at = now()
				WHERE scope = $1 AND user_id = $2 AND key = $3 AND status = 'IN_PROGRESS'
				  AND claimed_at <= now() - interval '`+leaseSQL+`' AND request_hash = $4
				  AND expires_at > now()
				RETURNING claim_generation, claimed_at`, k.Scope, user, k.Key, requestHash).Scan(&gen, &at)
		}
		if err == nil {
			return Result{Outcome: Proceed, Claim: Claim{Key: k, Generation: gen, ClaimedAt: at, RequestHash: requestHash}}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Result{}, err
		}
		// Lost a race: the row changed under us. Re-read.
	}
	return Result{Outcome: InProgress}, nil
}

// Fence must be the FIRST statement of the business transaction. It locks
// the claim row (lock rank 0) and proves the caller still owns the claim.
// ErrNotOwner means: roll back without any effect.
func Fence(ctx context.Context, tx pgx.Tx, c Claim) error {
	user, err := c.validate()
	if err != nil {
		return err
	}
	var one int
	err = tx.QueryRow(ctx, `SELECT 1 FROM idempotency_keys
		WHERE scope = $1 AND user_id = $2 AND key = $3
		  AND status = 'IN_PROGRESS' AND claim_generation = $4 AND request_hash = $5 AND claimed_at = $6
		FOR UPDATE`, c.Scope, user, c.Key.Key, c.Generation, c.RequestHash, c.ClaimedAt).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotOwner
	}
	return err
}

// Complete stores the deterministic response, in the same transaction as the
// business effect, just before COMMIT. ErrNotOwner means the claim is not the
// caller's (roll back). The effect must never commit without its record.
func Complete(ctx context.Context, tx pgx.Tx, c Claim, r Response) error {
	user, err := c.validate()
	if err != nil {
		return err
	}
	if r.Code < 100 || r.Code > 599 {
		return fmt.Errorf("idempotency: response code %d is not an HTTP status", r.Code)
	}
	if !json.Valid(r.Body) {
		return errors.New("idempotency: response body must be valid JSON")
	}
	tag, err := tx.Exec(ctx, `UPDATE idempotency_keys
		SET status = 'COMPLETED', response_code = $5, response_body = $6
		WHERE scope = $1 AND user_id = $2 AND key = $3
		  AND status = 'IN_PROGRESS' AND claim_generation = $4 AND request_hash = $7 AND claimed_at = $8`,
		c.Scope, user, c.Key.Key, c.Generation, r.Code, string(r.Body), c.RequestHash, c.ClaimedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotOwner
	}
	return nil
}

// Abandon gives up the caller's claim after a non-deterministic failure, so a
// retry can proceed at once instead of waiting out the lease. It does NOT
// delete the row. Deleting would let a fresh row restart at generation 1 and
// reuse fencing tokens. Instead it invalidates the token: the generation is
// bumped, and the lease is marked expired (claimed_at = -infinity), so the
// next Claim for the same request takes over immediately with a new
// generation. The key stays bound to its request: a different body is still a
// Mismatch (PDF p.28). It is fenced, so it never touches a completed record
// or a newer claim. It reports whether the claim was abandoned.
func (s *Store) Abandon(ctx context.Context, c Claim) (bool, error) {
	user, err := c.validate()
	if err != nil {
		return false, err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE idempotency_keys
		SET claim_generation = claim_generation + 1, claimed_at = '-infinity'
		WHERE scope = $1 AND user_id = $2 AND key = $3
		  AND status = 'IN_PROGRESS' AND claim_generation = $4 AND request_hash = $5 AND claimed_at = $6`,
		c.Scope, user, c.Key.Key, c.Generation, c.RequestHash, c.ClaimedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// Cleanup deletes up to limit records past their 24-hour retention. It never
// touches an unexpired record, and skips rows locked by an in-flight claimant
// (SKIP LOCKED), which a later run deletes. It returns the number deleted.
func (s *Store) Cleanup(ctx context.Context, limit int) (int64, error) {
	if limit < 1 {
		return 0, errors.New("idempotency: cleanup limit must be positive")
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM idempotency_keys
		WHERE (scope, user_id, key) IN (
			SELECT scope, user_id, key FROM idempotency_keys
			 WHERE expires_at <= now()
			 ORDER BY expires_at
			 LIMIT $1
			   FOR UPDATE SKIP LOCKED)`, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
