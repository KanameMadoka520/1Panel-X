---
status: partial
phase: 05-reproducible-release-and-vps-handoff
source: [05-VERIFICATION.md]
started: 2026-07-10T22:01:07-04:00
updated: 2026-07-10T22:01:07-04:00
---

## Current Test

number: 1
name: VPS prerequisites, checksum, and backup gate
expected: |
  Both checksum layers pass on a compatible disposable or snapshotted VPS, and a consistent pre-deployment backup exists.
awaiting: user response

## Tests

### 1. VPS prerequisites, checksum, and backup gate
expected: Both checksum layers pass on a compatible disposable or snapshotted VPS, and a consistent pre-deployment backup exists.
result: pending

### 2. Atomic replacement and service smoke test
expected: Agent and Core start in order without migration panic or restart loop, and core panel pages remain usable.
result: pending

### 3. Execute Phase 1 through Phase 4 UAT
expected: Theme, provider webhook, ClamAV/EICAR, and AI Agent capacity results are recorded in their persistent UAT files.
result: pending

### 4. Rollback rehearsal
expected: Previous binaries and data or the full VPS snapshot restore the official installation to a healthy state.
result: pending

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps
No gaps reported; human tests are pending.
