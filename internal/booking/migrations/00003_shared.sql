-- Shared tables every state-writing service owns (PDF p.21, via
-- docs/04-data/postgres-schema.md, whitespace-identical, verified).

-- +goose Up
CREATE TABLE idempotency_keys (
  scope         TEXT  NOT NULL,                -- operation, e.g. 'POST /holds'
  user_id       UUID  NOT NULL,
  key           TEXT  NOT NULL,
  request_hash  BYTEA NOT NULL,                -- same key + different body → 422
  status        TEXT  NOT NULL CHECK (status IN ('IN_PROGRESS','COMPLETED')),
  response_code INT,
  response_body JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at    TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (scope, user_id, key)
);

CREATE TABLE outbox (
  id              UUID PRIMARY KEY,
  topic           TEXT  NOT NULL,
  key             TEXT  NOT NULL,
  payload         BYTEA NOT NULL,
  status          TEXT  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','IN_FLIGHT','SENT','DEAD')),
  attempts        INT   NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by       TEXT,
  locked_until    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at         TIMESTAMPTZ
);
CREATE INDEX outbox_ready ON outbox (next_attempt_at) WHERE status IN ('PENDING','IN_FLIGHT');

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ NOT NULL DEFAULT now());

-- +goose Down
DROP TABLE processed_events;
DROP TABLE outbox;
DROP TABLE idempotency_keys;
