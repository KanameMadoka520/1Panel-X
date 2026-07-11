---
phase: 04-ai-agent-soft-limit
verified: 2026-07-10T20:46:26-04:00
status: human_needed
score: 4/5 must-haves verified
requirements:
  - AGENT-01
---

# Phase 4: AI Agent Soft Limit Verification Report

**Phase Goal:** AI Agent count is governed by host capacity and an optional operator setting rather than product license state.
**Verified:** 2026-07-10T20:46:26-04:00
**Status:** human_needed

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Missing or zero AIAgentLimit is unlimited and no license branch controls count | VERIFIED | `loadAIAgentLimit` returns zero for missing/blank values; Create checks only positive limits; xpack import and fixed five constant were removed. |
| 2 | Positive limits block at the configured count and updates accept only 0..1000 | VERIFIED | Capacity and setting validation implementation plus focused tests pass. |
| 3 | App Store metadata cannot restore a second AI count cap while normal app limits remain | VERIFIED | AI hooks set `SkipAppLimit`; nil/default hooks still enforce metadata limits; focused test passes. |
| 4 | Existing creation validation remains active and automated regression passes | VERIFIED | Changes are localized around capacity/install hooks; full agent tests pass. |
| 5 | Live creation beyond five and positive-limit blocking work on a real VPS under observed resource load | NEEDS HUMAN | No live multi-Agent acceptance evidence exists. |

**Score:** 4/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `agent/app/service/agents.go` | Optional soft-limit enforcement | VERIFIED | Uses setting value and preserves existing Create workflow. |
| `agent/app/service/setting.go` | Operator input validation | VERIFIED | Parses and normalizes integer range 0..1000. |
| `agent/app/service/app.go` | Scoped metadata bypass | VERIFIED | Normal enforcement is default; AI hook opts out internally. |
| `agent/app/service/agents_limit_test.go` | Limit regression evidence | VERIFIED | Covers unlimited, below/reached limit, and hook behavior. |

**Artifacts:** 4/4 verified

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| Agent settings API | Agent creation | `AIAgentLimit` repository key | WIRED | Create loads the current setting for each request. |
| Agent creation | App installer | `appInstallHooks{SkipAppLimit: true}` | WIRED | AI installs bypass metadata limit only. |
| Capacity error | Localized messages | `ErrAgentLimitReached` with `max` | WIRED | Configured maximum is passed to generalized translations. |

**Wiring:** 3/3 connections verified

## Requirements Coverage

| Requirement | Status | Blocking Issue |
|-------------|--------|----------------|
| AGENT-01: Unlimited default and optional operator soft limit | NEEDS HUMAN | Live beyond-five creation, positive-limit blocking, and resource observation have not run. |

**Coverage:** 0/1 requirements fully accepted; implementation and automated verification are complete.

## Automated Verification Passed

- Linux/WSL `agent`: `go test ./...`
- Combined `frontend`: `npm run build:pro`
- Commit author and committer identity verification

## Anti-Patterns and Residual Risk

- No global Pro/Enterprise state, bound license response, or public bypass flag was added.
- **Known race:** count and create are separate operations; concurrent creates can exceed a positive soft limit.
- **Known UX gap:** no dedicated frontend control exists for `AIAgentLimit`.

## Human Verification Required

### 1. Default unlimited behavior
**Test:** With AIAgentLimit missing or zero, create at least six Agents and continue to 10 or 25 only if host resources permit.
**Expected:** No fixed-five or App metadata limit error occurs; normal name, port, account, application, and lifecycle checks still apply.
**Why human:** Requires real Docker installs, ports, images, and host capacity.

### 2. Positive soft limit
**Test:** Set AIAgentLimit to a small positive value through the existing agent settings API, create to that count, then attempt one more.
**Expected:** Creation below the limit succeeds; creation at the limit is rejected with the configured maximum.
**Why human:** End-to-end settings persistence and installation require a running panel/agent.

### 3. Concurrent creation characterization
**Test:** Submit two creates concurrently when one slot remains.
**Expected:** Document whether both pass. An overshoot is a known soft-limit race, not an unexpected hidden guarantee.
**Why human:** Requires coordinated live requests and installation observation.

### 4. Resource guidance
**Test:** Monitor CPU, memory, disk, Docker, and port usage while increasing Agent count.
**Expected:** VPS documentation records practical limits and never equates software-unlimited with resource-unlimited.
**Why human:** Capacity depends on host size, images, models, and workloads.

## Gaps Summary

No automated implementation gap was found. Live capacity behavior is deferred. The non-atomic race and missing dedicated UI are explicit limitations, so status remains `human_needed`.

## Verification Metadata

**Verification approach:** Goal-backward implementation audit, focused capacity/hook tests, full agent tests, and combined frontend build evidence.
**Must-haves source:** `04-01-PLAN.md` and AGENT-01.
**Automated checks:** 4 categories passed, 0 failed.
**Human checks required:** 4.
**Known residual risk:** Concurrent overshoot and host resource exhaustion.

---
*Verified: 2026-07-10T20:46:26-04:00*
*Verifier: Codex retrospective phase audit*
