---
phase: 07-atomic-ai-agent-limit-ui
requirement: AGENT-02
verification_status: human_needed
automated: passed
environment: WSL Ubuntu, Go 1.26.1; Node 24.14.0 (/tmp/codex-node24.14.0)
date: 2026-07-10
---

# Phase 07 Verification: Atomic AI Agent Limit + Management UI

## Automated evidence (passed)

| Check | Command | Result |
|-------|---------|--------|
| Limiter + limit tests | `go test ./app/service -run 'AIAgent\|Limiter\|Capacity' -count=1` | ok |
| Concurrency stress | `go test ./app/service -run TestAIAgentLimiterConcurrentDoesNotExceed -count=300` | ok |
| Full agent regression | `go test ./... -count=1` | exit 0 (all packages ok) |
| gofmt / vet / build | changed files | clean / clean / ok |
| Frontend ESLint | `eslint` on changed files | clean (no findings) |
| Frontend production build | `npm run build:pro` (WSL, Node 24.14.0) | ✓ built in ~50s |

### Test coverage mapped to acceptance criteria

- **AC1 (positive limit never exceeded; slot freed on failure; inFlight drains):** `TestAIAgentLimiterConcurrentDoesNotExceed` (32 creators vs limit 3: max committed never exceeds 3, fills exactly 3, inFlight ends 0; stressed 300x); `TestAIAgentLimiterReleaseOnFailure` (released-without-commit frees the slot; rejected once committed).
- **AC2 (unlimited when 0/absent; 1..1000 semantics unchanged):** `TestAIAgentLimiterUnlimited` (limit 0 reserves nothing, never queries count); existing `TestValidateAIAgentCapacity*` and `TestSettingServiceUpdateAIAgentLimit*` still pass.
- **AC3 (no lock across install; count read under guard):** by construction (`reserve` holds `l.mu` only for `countFn` + check; `Create` reserves before `installWithHooks` and releases via `defer` after `agentRepo.Create`).
- **AC4 (UI reads/writes via existing endpoint):** `limit/index.vue` calls `getAgentSettingInfo` / `updateAgentSetting({key:'AIAgentLimit'})`; production build + ESLint pass. Live interaction is human UAT.

## Security review

- The limiter is an operational soft limit, not a security boundary; the fix is a correctness (race) fix.
- No new endpoint or secret; the UI uses the existing agent-setting endpoint whose server-side validation (0..1000, `AgentSettingUpdate` key allowlist) is unchanged.
- The reservation is process-local to the single agent, which is the correct scope for a single-process count guard.

## Human-needed (see 07-HUMAN-UAT.md)

Live creation beyond the former limit of five, a positive-limit block under real concurrent requests on a VPS, the browser drawer interaction, and host resource observation.
