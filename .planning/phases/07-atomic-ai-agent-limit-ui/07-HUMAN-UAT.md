---
phase: 07-atomic-ai-agent-limit-ui
requirement: AGENT-02
status: pending
items: 3
---

# Phase 07 Human UAT: Atomic AI Agent Limit + Management UI

Require a running panel on a VPS with enough host capacity to create several agents. Do not mark pass without evidence.

## UAT-07-1: UI set/read + unlimited default
**Steps:** Open AI Agents → "Count Limit". Confirm the drawer shows the current value (0 by default). Set it to 0 (unlimited) and create a 6th+ agent; then set a positive value and reopen the drawer.
**Expected:** with 0, creation beyond five succeeds (subject to host resources); the drawer persists and re-reads the value through the settings API; the helper note about host resources is shown.
**Result:** _pending_

## UAT-07-2: Positive limit blocks at the configured count (single + concurrent)
**Steps:** Set the limit to N (e.g. 3). Create agents sequentially until rejected. Then, from a script, fire several create requests concurrently while at N-1 committed.
**Expected:** sequential creation is blocked at N with the localized limit error; concurrent requests never produce more than N committed agents (the reservation rejects the excess).
**Result:** _pending_

## UAT-07-3: Resource observation
**Steps:** While creating multiple agents, observe host CPU, memory, disk, ports, and Docker container count.
**Expected:** "unlimited count" is confirmed to not imply unlimited host resources; document the practical ceiling on the test VPS.
**Result:** _pending_
