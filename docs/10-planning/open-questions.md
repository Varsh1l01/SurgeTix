# Open Questions (from the PDF)

*Source: PDF p.37 ("Open questions"), transcribed in full. These are questions the **PDF author already raised**. Gaps found while documenting are tracked separately in [ambiguities.md](ambiguities.md).*

Status values: **Open** (no decision) · **Proposed** (the PDF suggests an answer) · **Decided** (owner has approved; record the date and ADR).

| ID | Question (PDF wording) | PDF proposal | Status | Blocks phase | Affects |
|---|---|---|---|---|---|
| **OQ-1** | Refund policy defaults: 100 % until 7 days out, 50 % until 24 hours, none after? Or let each event choose freely? | Example policy appears in FR-13 and flow 5 | **Open** | 7 (Lifecycle); schema in 4 (Catalog) | Catalog event config, `QuoteCancellation` |
| **OQ-2** | Proof-of-work difficulty: about 1 s on a mid-range phone; tune after measuring on real devices. | ~1 s on a mid-range phone | **Proposed** (measure first) | 5 (Waiting room) | `queue/challenge`, T10 |
| **OQ-3** | Postgres access: `pgx` with hand-written SQL, or `sqlc` code generation? | **`sqlc` for CRUD, hand-written SQL for the hold, GA and refund paths** | **Proposed**. Note the Tech stack (p.14) already lists `pgx + sqlc` ([AMB-36](ambiguities.md#amb-36)) | 1 (Foundations) | All Go data access; ADR-024 |
| **OQ-4** | Should a ticket bought through the second-chance queue be transferable, or locked to the buyer? | — | **Open** | 7 (Lifecycle) | `TransferTicket`, tickets schema (no origin flag exists today) |
| **OQ-5** | Hosting: demo video and local run only, or a small cloud VM during application season? | — | **Open** | 9 (Ship) | Edge (NGINX vs Cloudflare), cost metric |
| **OQ-6** | Load generation: which cloud VMs for the 50,000-user k6 run, and what budget? | 2–3 cloud VMs (p.10) | **Open** | 5 (T2) | T2, T3, report |
| **OQ-7** | Should the naive baseline be a single monolith (simplest) or the same services without the waiting room and atomic holds (fairer comparison)? | — | **Open** | **2 (Naive baseline)**, the earliest blocker | T6, the whole comparison report |

## Decision order

*Derived from "Blocks phase" above:* **OQ-3** and **OQ-7** must be settled before phases 1–2 begin. **OQ-2**, **OQ-6** by phase 5. **OQ-1**, **OQ-4** by phase 7. **OQ-5** by phase 9.

## Recording a decision

When an OQ is decided:
1. Set Status to **Decided (YYYY-MM-DD)**.
2. Add or update the ADR in [`/ARCHITECTURE_DECISIONS.md`](../../ARCHITECTURE_DECISIONS.md).
3. Update every doc that links the OQ.
4. Note it in [`/PROGRESS.md`](../../PROGRESS.md).
