# FakePay Simulator

*Source: PDF p.1, p.2 (non-goals), p.10 (diagram), p.12 (data flow), p.14 (tech stack: Payments), p.16, p.29–30 (flows 3, 5), p.32–34 (failures), p.35 (T5).*

## Purpose

FakePay **plays the external payment provider**. *"Payments are simulated with a fake provider that behaves like a real one; no real money moves"* (p.1). It is the PDF's own **Go simulator** (p.14), not one of the six services, and it is treated as **external**: Checkout talks to it as it would talk to Stripe or Razorpay.

## Why a simulator (p.14)

> *Latency, failures and lost webhooks on demand, at load.*
> Stripe or Razorpay test mode: realistic but rate-limited and **cannot fail on command**; can be swapped in behind the same interface later.

So **Checkout must depend on a provider interface**, not on FakePay specifics.

## Capabilities named in the PDF

| Capability | Source |
|---|---|
| **Payment intents** (created per order, keyed by `order_id`) | p.14, p.29 |
| **Hosted payment page** where the buyer pays (`payment_url` returned by checkout) | p.12, p.26 |
| **Signed webhooks** (HMAC) for payment and refund results, sent to `POST /v1/webhooks/fakepay` via the Gateway | p.14, p.26, p.29 |
| **Refunds**, keyed by `refund_id` | p.14, p.30 |
| **Idempotency keys** | p.14 |
| **Configurable delays and failures** | p.16 |
| **Lost/duplicated webhooks on demand** | p.14, p.32 |
| **Status polling** (the reconciler polls FakePay for anything pending > 2 min) | p.32 |
| **Totals for reconciliation** ("ledger totals match FakePay") | p.7 |

## Fault modes required by tests

| Test | Behaviour FakePay must produce |
|---|---|
| **T5** | +3 s latency, 20 % failures, then **fully down for 5 minutes** |
| Failure table | Webhook **lost** or **duplicated** |
| Refund-provider outage | Refund calls fail until recovery; refunds then complete within 15 min |

## Latency SLO it influences

Payment completion (buyer submits → signed webhook received) **p99 < 2 s** on a **healthy** FakePay (provider-controlled).

## Not specified

The PDF lists the capabilities but **not the FakePay API** (endpoints, payloads, signature scheme details, how faults are configured, how the totals are exposed). See [AMB-31](../10-planning/ambiguities.md#amb-31). Whether the "payment method" used for purchase caps (FR-18) exists in FakePay is also open ([AMB-10](../10-planning/ambiguities.md#amb-10)).
