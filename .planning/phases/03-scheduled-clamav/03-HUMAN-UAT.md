---
status: partial
phase: 03-scheduled-clamav
source: [03-VERIFICATION.md]
started: 2026-07-10T21:06:24-04:00
updated: 2026-07-10T21:06:24-04:00
---

## Current Test

number: 1
name: Restart restoration
expected: |
  An enabled disposable rule receives a new nonzero entry ID after agent restart and fires once per configured schedule.
awaiting: user response

## Tests

### 1. Restart restoration
expected: An enabled disposable rule receives a new nonzero entry ID after agent restart and fires once per configured schedule.
result: pending

### 2. Manual versus scheduled overlap
expected: A scheduled trigger during a long manual scan does not start a second scanner for the same rule.
result: pending

### 3. Isolated EICAR quarantine
expected: ClamAV detects the EICAR file in a disposable directory and confines the copy or move to the restrictive quarantine tree.
result: pending

### 4. Lifecycle operations on the VPS
expected: Update, disable, enable, and delete do not leak cron entries; mutations are refused while the rule is running.
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
