---
phase: 03-scheduled-clamav
verified: 2026-07-10T20:46:26-04:00
status: human_needed
score: 5/6 must-haves verified
requirements:
  - CLAM-01
---

# Phase 3: Scheduled ClamAV Verification Report

**Phase Goal:** ClamAV rules can run on durable schedules without overlapping scans or unsafe path and deletion behavior.
**Verified:** 2026-07-10T20:46:26-04:00
**Status:** human_needed

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Create/update/enable/disable/delete compensate database and cron failures | VERIFIED | Lifecycle implementation and focused tests pass. |
| 2 | Enabled schedules restore after startup and receive new entry IDs | VERIFIED | Startup hook, restoration implementation, and restore tests pass. |
| 3 | Manual and scheduled scans of one rule cannot overlap | VERIFIED | Conditional `is_executing` update and atomic claim test pass. |
| 4 | Scan/quarantine/removal targets reject roots, escapes, symlinks, overlap, and unsafe names | VERIFIED | Service/util validation and security tests pass; created quarantine directories use `0700`. |
| 5 | Community users can view and edit schedules and the frontend builds | VERIFIED | License gates removed from schedule controls; targeted ESLint and build pass. |
| 6 | A real VPS restores schedules and safely detects/quarantines EICAR | NEEDS HUMAN | No live ClamAV/VPS/EICAR evidence exists. |

**Score:** 5/6 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `agent/app/service/clam.go` | Durable lifecycle and run guard | VERIFIED | Substantive create/update/restore/status/delete/claim logic. |
| `agent/utils/clam/clam.go` | Cron callback and safe quarantine | VERIFIED | Standard cron validation, registered handler, 0700 directories, confined removal. |
| `agent/cron/cron.go` | Restart recovery | VERIFIED | Calls restore before starting global cron. |
| Clam schedule/security tests | Negative-path evidence | VERIFIED | Focused lifecycle, concurrency, and path-safety cases exist and pass. |

**Artifacts:** 4/4 verified

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| Agent cron startup | Clam service | `RestoreClamSchedules` | WIRED | Enabled rules are re-registered before cron starts. |
| Cron callback | Rule execution | Registered `HandleOnce` handler | WIRED | Callback carries persisted rule ID. |
| Manual/scheduled start | Database guard | Conditional GORM update | WIRED | Only one caller can claim a non-running rule. |
| Clam scan task | Quarantine directory | `PrepareInfectedDirectory` | WIRED | Move/copy target is constructed under validated base/rule/run components. |

**Wiring:** 4/4 connections verified

## Requirements Coverage

| Requirement | Status | Blocking Issue |
|-------------|--------|----------------|
| CLAM-01: Durable safe scheduled ClamAV | NEEDS HUMAN | Restart, real cron timing, Clam daemon, and isolated EICAR acceptance have not run. |

**Coverage:** 0/1 requirements fully accepted; implementation and automated verification are complete.

## Automated Verification Passed

- Linux/WSL `agent`: `go test ./...`
- Targeted ESLint for Phase 3 frontend changes
- `frontend`: `npm run build:pro`
- Commit author and committer identity verification

## Anti-Patterns Found

No ID-zero schedule registration, `0777` quarantine creation, direct user-supplied `RemoveAll` target, root scan target, overlap allowance, or non-atomic in-memory-only run lock was found.

## Human Verification Required

### 1. Restart restoration
**Test:** Create and enable a short disposable schedule, record its entry ID, restart the agent, and observe the rule and database.
**Expected:** The rule remains enabled, receives a new nonzero entry ID, and fires once per schedule.
**Why human:** Requires process restart and real cron timing.

### 2. Manual versus scheduled overlap
**Test:** Start a long scan and let a high-frequency schedule fire for the same rule.
**Expected:** Only one scan runs; the second attempt reports already executing and creates no duplicate scanner.
**Why human:** Requires a real long-running Clam process and process inspection.

### 3. Isolated EICAR quarantine
**Test:** In a disposable directory, create the standard EICAR file and run a `move` or `copy` rule whose quarantine base is outside the scan tree.
**Expected:** Detection is recorded, the file is quarantined below `1panel-infected/<rule>/<run>`, and directory permissions are restrictive.
**Why human:** Requires an installed ClamAV engine and safe malware-test handling.

### 4. Lifecycle operations on the VPS
**Test:** Update, disable, enable, and delete the disposable rule, including an attempted mutation while it runs.
**Expected:** Cron entries do not leak; running mutations are refused; deletion removes only the rule quarantine directory.
**Why human:** Requires scheduler and filesystem observation on the target host.

## Gaps Summary

No automated implementation gap was found. Host integration and EICAR acceptance are deferred, so status remains `human_needed`.

## Verification Metadata

**Verification approach:** Goal-backward implementation audit, focused lifecycle/security tests, full agent tests, frontend lint, and build evidence.
**Must-haves source:** `03-01-PLAN.md` and CLAM-01.
**Automated checks:** 5 categories passed, 0 failed.
**Human checks required:** 4.
**Known residual risk:** Host-specific ClamAV, cron timing, permissions, and scan duration.

---
*Verified: 2026-07-10T20:46:26-04:00*
*Verifier: Codex retrospective phase audit*
