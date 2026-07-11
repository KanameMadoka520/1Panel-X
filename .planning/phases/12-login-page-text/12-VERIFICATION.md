---
phase: 12-login-page-text
requirement: LOGIN-TEXT-01
verification_status: human_needed
automated: passed
security_review: passed (0 confirmed defects; workflow wf_5e417e7b-d27, 11 agents)
environment: WSL Ubuntu, Go 1.26.1, Node 24.14.0
date: 2026-07-11
---

# Phase 12 Verification: Open Login-Page Text (+ login-hero fix)

## Automated evidence (passed)

| Check | Command | Result |
|-------|---------|--------|
| Focused enhancement tests | `go test ./app/service -run 'Enhancement\|Branding\|LoginText' -count=1` | ok |
| Full core regression | `go test ./...` (core) | exit 0 (67 ok) |
| gofmt / vet / build | changed files / `go vet` / `go build` | clean |
| Frontend build | `npm run build:pro` (Node 24.14.0) | passed |
| Changed-file ESLint | 9 changed FE files | 0 errors |

### Mapped to acceptance criteria
- **AC1/AC2 (writable/readable, reject-`<>`/control chars, caps, fail-closed):** `TestValidateEnhancementBrandingFields` accepts empty/valid/CJK and rejects markup, control char, over-length, and **bidi-override / isolate** for the login-text keys; `login-form.vue` renders via `{{ }}` (no `v-html`).
- **AC3 (strict anon subset):** `TestPublicEnhancementSettingIsStrictSubset` includes `loginWelcome/loginSubtitle/copyright` in the public set and still excludes watermark/paths; `TestEnhancementLoginTextPublicFallsBack` proves a corrupt/bidi stored value falls back to `""` on the anon surface.
- **AC4 (LOGIN-HERO fix):** `login/index.vue` preloads reactively (`watch` on `themeConfig`, immediate) with a generation guard so stale async callbacks cannot clobber the latest run.
- **AC5:** full core regression + build + ESLint + adversarial review below.

## Security review (passed)
Workflow `wf_5e417e7b-d27` — 4 lenses (login-text XSS T7, anon-leak T8, login-hero-fix correctness, logic-regression) + completeness critic → per-finding adversarial verify (11 agents). **0 confirmed defects**; all 6 raw findings refuted as not-a-bug. Three were adopted as quality hardening:
- **Generation guard** in `applyLoginBranding` — stops a stale default-bg `img.onload` from clobbering a configured color background (the race the reactive fix could introduce).
- **`validateBrandingText` now also rejects `unicode.Bidi_Control`** — Trojan-Source-style bidi reordering can't reach the pre-auth text (all branding text: Title/MasterAlias/LoginWelcome/…).
- **Public-surface fallback test** for the three login-text fields.

## Human-needed (12-HUMAN-UAT.md)
Browser: login welcome/subtitle/copyright render on the login page and markup is rejected; uploaded login image/background now display (LOGIN-HERO fix).

## Note
Running full core `go test ./...` regenerates `x-log.json` (swagger side-effect); reverted. The release build uses focused/compile-only tests.
