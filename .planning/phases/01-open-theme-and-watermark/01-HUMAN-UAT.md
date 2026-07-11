---
status: partial
phase: 01-open-theme-and-watermark
source: [01-VERIFICATION.md]
started: 2026-07-10T21:06:24-04:00
updated: 2026-07-10T21:06:24-04:00
---

## Current Test

number: 1
name: Public login theme and data minimization
expected: |
  The anonymous response contains only theme and themeColor, and the login page applies them without exposing watermark data.
awaiting: user response

## Tests

### 1. Public login theme and data minimization
expected: The anonymous response contains only theme and themeColor, and the login page applies them without exposing watermark data.
result: pending

### 2. Theme persistence and system mode
expected: Custom and preset colors survive refresh in light, dark, and system modes, including an operating-system theme change.
result: pending

### 3. Watermark rendering and routed state
expected: Enabling, editing, and disabling the watermark preserves the active route and page state while rendering the configured appearance.
result: pending

### 4. Target-browser CSS color mixing
expected: Primary hover and light shades remain readable and consistent in the supported desktop browsers.
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
