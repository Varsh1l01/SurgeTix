# Identity Service

*Source: PDF p.5 (FR-1), p.14 (tech stack: Auth), p.15 (responsibilities), p.22 (owned tables), p.25–26 (REST, gRPC).*

## Purpose

Identity owns **who a user is** and **the keys that prove it**. It is a **security boundary**: JWT signing keys **never leave it** (p.15).

## Responsibilities

- **Sign-up and sign-in with email/password**, hashed with **bcrypt**.
- **Google OAuth with PKCE** via `golang.org/x/oauth2`.
- **Email verification.** Required **before joining a queue** (FR-1) and for the recipient of a transfer ("verified account", FR-16).
- **JWT issuance (RS256, `golang-jwt`)** and **refresh**.
- **JWKS publication.** The Gateway caches the keys (p.11).
- **User lookup by email** for ticket transfers.

Keycloak (heavy to run locally) and Auth0/Firebase (hosted, hides the work) were rejected (p.14).

## Owned data

Tables named by the PDF (p.22): `users`, `credentials`, `oauth_accounts`, `signing_keys`.

> **Columns are not specified.** No DDL is written here, to avoid inventing a schema. See [AMB-13](../10-planning/ambiguities.md#amb-13). The schema will need the shared `idempotency_keys` table if Identity endpoints are treated as state-changing under FR-12 (sign-up and verify-email are writes).

## Public API (via Gateway)

| Method and path | Auth | Purpose | Responses |
|---|---|---|---|
| `POST /v1/auth/signup` | none | Create account | `200` tokens, `401` |
| `POST /v1/auth/login` | none | Email/password sign-in | `200` tokens, `401` |
| `POST /v1/auth/refresh` | refresh token | New access JWT | `200` tokens, `401` |
| `POST /v1/auth/verify-email` | none | Confirm email | `200` tokens, `401` (shared row in the PDF) |
| `GET /v1/auth/google/start` | none | Begin OAuth + PKCE | `302` |
| `GET /v1/auth/google/callback` | none | Complete OAuth | `302` |

The PDF lists the four `POST` routes on one row with the shared responses "200 tokens, 401" (p.25).

## Internal gRPC — `identity.v1`

| RPC | Caller | Purpose |
|---|---|---|
| `GetJWKS` | Gateway | Public signing keys (cached) |
| `VerifyToken` | Gateway | Token verification |
| `LookupUserByEmail` | Booking (transfers) | Resolve a transfer recipient. Must be a verified account |

## Scaling

**Scales on:** sign-ins, which are **low** (p.15). Buyers sign in *before* the sale, and the hot path verifies JWTs from the Gateway's cached keys.

## Events

None in the PDF's topic table. Identity publishes nothing to Kafka in v1 as specified.

## Open items

- [AMB-12](../10-planning/ambiguities.md#amb-12): access/refresh token TTLs; JWKS rotation and cache TTL.
- [AMB-13](../10-planning/ambiguities.md#amb-13): table columns.
- [AMB-14](../10-planning/ambiguities.md#amb-14): how *admin* and *staff* roles are represented and granted.
- [AMB-18](../10-planning/ambiguities.md#amb-18): who sends the **verification** email (Mailpit is specified for ticket and refund emails only).
- [AMB-19](../10-planning/ambiguities.md#amb-19): signing-key storage and rotation.
- [AMB-24](../10-planning/ambiguities.md#amb-24): whether Google sign-ins count as email-verified.
- [AMB-37](../10-planning/ambiguities.md#amb-37): what "AuthPlatform" refers to.
