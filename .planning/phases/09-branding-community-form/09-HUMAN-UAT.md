---
phase: 09-branding-community-form
requirement: BRAND-02
status: pending
items: 2
---

# Phase 09 Human UAT: Community Branding Form

Requires a running panel with a browser. Do not mark pass without evidence.

## UAT-09-1: Set and persist branding
**Steps:** Open Panel Settings → "Brand & Login". Set brand text, master alias, login background type = color + a color, and a button/link color. Reload the page.
**Expected:** each value persists (reloads from the settings API); invalid input (e.g. markup in brand text) is rejected with an error.
**Result:** _pending_

## UAT-09-2: Pre-auth login render
**Steps:** Log out. On the login page (unauthenticated), observe the brand text and login colors.
**Expected:** the login page renders the configured brand text and colors before authentication; text renders literally (no HTML execution).
**Result:** _pending_
