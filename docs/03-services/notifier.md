# Notifier (Worker)

*Source: PDF p.7 (FR-19, FR-21), p.10 (diagram), p.12 (data flow phase 4), p.15 (SSE, Mailpit), p.16 (responsibilities), p.24 (topic consumers), p.30–31 (flows 3, 5, 6).*

## Purpose

The Notifier keeps **slow side effects off the critical path** (p.16). It is a **worker**, not a request-serving service. It **scales on Kafka lag**.

## Responsibilities

| Responsibility | Trigger | Channel |
|---|---|---|
| **Ticket confirmation email** with the QR ticket | `OrderConfirmed` (`checkout.orders`) and `TicketsIssued` (`booking.tickets`) | Email via **Mailpit** (local SMTP inbox) |
| **Refund receipt email** | Refund events on `checkout.orders` | Email via Mailpit |
| **SSE seat-map deltas** | `InventoryChanged` (`booking.inventory`) | SSE: deltas, **not full maps** (NFR-EFF) |
| **Second-chance offers** | Offer issued by the Waiting Room | **SSE or email** (p.31) |

The data-flow diagram shows *"Notifier: SSE seat delta, ticket email (Mailpit)"* returning to the buyer app (p.12).

## Events consumed

| Topic | Events used | Source |
|---|---|---|
| `booking.holds` | Lifecycle events (Notifier is a listed consumer; the use is not specified) | p.24 |
| `booking.inventory` | InventoryChanged (SSE deltas) | p.24 |
| `booking.tickets` | TicketsIssued, … | p.24 |
| `checkout.orders` | OrderConfirmed, RefundIssued, … | p.24 |

Every consumer is idempotent: `processed_events(event_id)` dedups, and stale aggregate versions are ignored (p.24).

> How the Notifier learns of a second-chance offer (a Kafka event from the Waiting Room, or a direct call) is not specified. No Waiting Room topic exists in the topic table. See [AMB-07](../10-planning/ambiguities.md#amb-07).

## Degradation

*"Graceful degradation if notifications, analytics or SSE fail"* (NFR-AVL). A Notifier outage delays emails and SSE but never affects holds, payments or confirmations. Kafka retains the events for replay.

## Technology

- **SSE** chosen over WebSockets (two-way not needed, more connection state) and polling (wasteful at this scale) (p.15).
- **Mailpit** chosen over SendGrid/SES (real delivery not needed; swappable) (p.15).

## Open items

- [AMB-07](../10-planning/ambiguities.md#amb-07): offer-delivery path.
- [AMB-08](../10-planning/ambiguities.md#amb-08): the Notifier owns *"SSE seat-map deltas"*, but the public route `GET /seats/stream` sits under the Gateway and Booking exposes `WatchSeats`. Which process holds the SSE connections?
- [AMB-16](../10-planning/ambiguities.md#amb-16): "analytics" consumer.
- [AMB-18](../10-planning/ambiguities.md#amb-18): verification email sender.
