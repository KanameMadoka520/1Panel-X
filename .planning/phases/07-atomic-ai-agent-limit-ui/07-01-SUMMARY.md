---
phase: 07-atomic-ai-agent-limit-ui
requirement: AGENT-02
plan: 07-01
status: implemented_automated_verified
human_uat: pending
completed: 2026-07-10
---

# Phase 07 Summary: Atomic AI Agent Limit + Management UI

## What shipped

Two disclosed debts of the v1.0 AI Agent soft limit (AGENT-01) are closed:

1. **Race-free enforcement.** The v1.0 code read the count then created the agent after the full container install, with no lock spanning the two, so concurrent creations could exceed a positive `AIAgentLimit`. It is replaced by a mutex-guarded slot reservation (`aiAgentLimiter`) that reads the committed count inside the lock (reflecting every prior commit) and tracks an `inFlight` counter of reserved-but-uncommitted creations, rejecting when `count+inFlight` would reach the limit. The lock is held only for the count query and check, never across the install.
2. **Management UI.** A "Count Limit" drawer on the AI agents list page reads and writes `AIAgentLimit` through the existing validated agent-setting endpoint.

## Commits

- `685ca0a18` feat: make AI agent limit enforcement race-free
- `9864d6292` test: verify AI agent limit reservation under concurrency
- `f8e7b790a` feat: add AI agent count limit management UI

## Files

- `agent/app/service/agents.go` — `aiAgentLimiter` type, `reserve`, and `reserveAIAgentSlot`; `Create` now reserves a slot (`defer release`) instead of the racy check-then-act. `BatchInstall` inherits it via `Create`.
- `agent/app/service/agents_limit_test.go` — unlimited, concurrent-never-exceeds (32 goroutines vs limit 3, stressed `-count=300`), and release-on-failure tests.
- `frontend/src/views/ai/agents/agent/limit/index.vue` (new) — the 0..1000 limit drawer.
- `frontend/src/views/ai/agents/agent/index.vue` — "Count Limit" toolbar button + wiring.
- `frontend/src/api/interface/setting.ts` — `aiAgentLimit` on `AgentSettingInfo`.
- `frontend/src/lang/modules/{zh,en}.ts` — i18n strings under `aiTools.agents`.

## Decisions realized

- D-01..D-04 (reservation, count-inside-lock, unlimited no-op, injectable core) — implemented and proven by the concurrency test.
- D-05..D-07 (UI on the existing validated endpoint; no new backend endpoint; existing validation and App-Store bypass unchanged) — implemented.

## Correctness argument (proven by test)

At reservation time `committed + inFlight <= limit` holds after each grant (count read fresh under the lock). A reservation is released only after `agentRepo.Create` (via `defer`), so a granted slot always covers its own commit and `committed` never exceeds the limit; a failed install releases without committing, freeing the slot. The 32-goroutine test asserts the committed count never exceeds the limit, fills exactly the limit, and drains `inFlight` to zero (300 iterations).

## Tech debt / not done

- No `-race` run (no cgo toolchain in this environment); shared state is confined to `l.mu` and the invariant test passes deterministically.
- i18n added for zh + en only; other locales fall back.
- Per-resource (CPU/mem/disk/GPU) admission control is out of scope.
- Human UAT (live multi-request race on a VPS, browser interaction, resource observation) is pending — see `07-HUMAN-UAT.md`.
