-- APPROVED SCHEMA DEVIATION (owner, 2026-11-06; ADR-036 amendment, AMB-48).
-- The ONLY change to the PDF DDL: two ADDITIVE columns on idempotency_keys.
-- Every PDF column, the primary key and the CHECK stay unchanged.
--   claim_generation: fencing token. Bumped on every (re-)claim. An old
--                     claimant's fence/complete/abandon must match it, so it can
--                     never overwrite, complete or delete a newer claim.
--   claimed_at:       start of the current claim's 30 s lease.

-- +goose Up
ALTER TABLE idempotency_keys
  ADD COLUMN claim_generation BIGINT      NOT NULL DEFAULT 1,
  ADD COLUMN claimed_at       TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE idempotency_keys
  DROP COLUMN claimed_at,
  DROP COLUMN claim_generation;
