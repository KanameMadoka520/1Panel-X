---
phase: 08-branding-backend-wiring
requirement: BRAND-01
verification_status: human_needed
automated: passed
environment: WSL Ubuntu, Go 1.26.1 (/tmp/codex-go1.26.1)
date: 2026-07-11
---

# Phase 08 Verification: Branding Backend Wiring

## Automated evidence (passed)

| Check | Command | Result |
|-------|---------|--------|
| Focused enhancement tests | `go test ./app/service -run 'Enhancement\|Branding\|Public' -count=1` | ok |
| Full core regression | `go test ./... -count=1` (core) | exit 0 |
| gofmt / vet / build | changed files / `go build ./...` (core) | clean / clean / ok |

### Mapped to acceptance criteria
- **AC1/AC4 (writable/readable, fail-closed, corrupt fallback):** `TestValidateEnhancementBrandingFields` (accept incl. empty; the `default` still rejects unknown keys), `TestEnhancementBrandingStoredValueFallsBack` (corrupt values fall back to empty).
- **AC2 (XSS/enum/color validation):** `TestValidateEnhancementBrandingFields` rejects angle brackets, control chars, over-length, bad bg-type enum, and non-color backgrounds/button colors.
- **AC3 (strict anon subset):** `TestPublicEnhancementSettingIsStrictSubset` — public payload = exactly the cosmetic set, ⊆ authed, excludes watermark and image keys.
- **AC5:** full core `go test ./...` exit 0.

## Security review
- The pre-auth XSS control is server-side (reject `<>`/control chars): no markup can ever be persisted, so the login page cannot render injected script regardless of client rendering.
- The anon surface is bounded by a separate DTO + a subset-assertion test; no watermark, image bytes/paths, versions, or secrets leak.
- No new route, no file write, no license state change.

## Human-needed (08-HUMAN-UAT.md)
Browser check that the login page renders custom brand text and login colors before authentication, and that a value with markup is rejected end to end.

## Note
Running the full core `go test ./...` regenerates `core/cmd/server/docs/x-log.json` (a test side-effect in `swagger_test.go`). This was reverted; the release build uses focused tests only and does not regenerate it.
