# Phase 07: Atomic AI Agent Limit + Management UI - Context

**Gathered:** 2026-07-10
**Milestone:** v1.1 Open Enhancement Hardening
**Requirement:** AGENT-02

<domain>
## Phase Boundary

This phase closes the two disclosed debts of the v1.0 AI Agent soft limit (AGENT-01): (1) the count-then-create enforcement is a non-atomic race, and (2) there is no dedicated UI to view/set `AIAgentLimit`. It makes enforcement race-free with a mutex-guarded slot reservation and adds a management control bound to the existing validated setting endpoint.

It does NOT change the limit semantics (missing/0 = unlimited, 1..1000 = soft cap), does not add resource (CPU/mem/disk/GPU) enforcement, does not change license state, and does not alter the App Store metadata bypass scoping.
</domain>

<decisions>
## Implementation Decisions

### Concurrency model
- **D-01:** Replace the `Create` capacity check (`agents.go:146-155`) with a `reserveAIAgentSlot()` call whose returned `release` is deferred. The reservation is a process-level guard, not a DB transaction, because the commit (`agentRepo.Create`) happens only after the slow `installWithHooks`.
- **D-02:** The limiter reads the committed count **inside** its mutex (so it reflects all prior commits) and adds an `inFlight` counter of reserved-but-uncommitted creations. It rejects when `count + inFlight >= limit`. This never under-counts, so a positive limit can never be exceeded, while the lock is held only for the count query + check (never across the install).
- **D-03:** `limit <= 0` (unlimited) reserves nothing and returns a no-op release. `release` is idempotent (`sync.Once`) and only decrements `inFlight`; the committed row is already reflected by the DB count.
- **D-04:** The limiter core is injectable (`reserve(limit int64, countFn func() (int64, error))`) so a concurrency test can hammer it with a fake committed-counter and assert the cap holds without real Docker installs.

### UI
- **D-05:** Add a numeric control (0 = unlimited, 1..1000) to the existing AI Agent settings surface, calling the existing setting-update API (`SettingService.Update` key `AIAgentLimit`, already validated 0..1000 at `setting.go:128`). No new backend endpoint. Reads current value via the existing setting search/get.
- **D-06:** Frontend change is verified by `npm run build:pro` and changed-file ESLint; live browser interaction is human UAT.

### Compatibility
- **D-07:** Keep existing name/port/account/model/compose validation and the App-Store metadata bypass exactly as-is. `BatchInstall` routes through the same `Create`, so it inherits the reservation.
</decisions>

<specifics>
## Specific Ideas

- Correctness target: with a positive limit N and G concurrent creates, committed agents never exceed N; failed installs release their slot; `inFlight` returns to 0 at rest.
- Brief-sanctioned wording: "concurrency slot reservation OR transactional creation limit" — this uses slot reservation because the create is not a single short transaction.
- "Unlimited" still means no software count cap, not unlimited host resources.
</specifics>

<canonical_refs>
## Canonical References

- `.planning/REQUIREMENTS.md` - AGENT-02 acceptance criteria.
- `agent/app/service/agents.go:135-333` - `Create`, `loadAIAgentLimit`, `validateAIAgentCapacity`.
- `agent/app/service/setting.go:126-135` - existing `AIAgentLimit` 0..1000 validation.
- `.planning/phases/04-ai-agent-soft-limit/04-CONTEXT.md` - the v1.0 soft-limit decisions this phase hardens (D-09 non-atomic, D-08 no UI).
</canonical_refs>

<deferred>
## Deferred Ideas
- Per-resource (CPU/mem/disk/GPU) admission control.
- DB-level unique/constraint enforcement (the reservation is sufficient for a single-process agent).
</deferred>

---
*Phase: 07-atomic-ai-agent-limit-ui*
