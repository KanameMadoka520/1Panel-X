---
phase: 10-branding-image-upload
requirement: BRAND-IMG-01
verification_status: human_needed
automated: passed
security_review: passed (0 confirmed defects, workflow wf_cdaea91b-aad, 15 agents)
environment: WSL Ubuntu, Go 1.26.1 (/tmp/codex-go1.26.1)
date: 2026-07-11
---

# Phase 10 Verification: Branding Image Upload (backend + serve hardening)

## Automated evidence (passed)

| Check | Command | Result |
|-------|---------|--------|
| Focused asset + enhancement tests | `go test ./app/service -run 'Enhancement\|Asset\|Branding\|IsBrandingAssetFileName' -count=1` | ok |
| Full core regression | `go test ./...` (core) | exit 0 |
| gofmt / vet / build | changed files / `go vet` / `go build ./...` (core) | clean / clean / ok |

### Mapped to acceptance criteria
- **AC1 (serve hardening):** `TestIsBrandingAssetFileName` (enum gate); `sniffImageContentType` allowlist + `nosniff` in `RegisterImages`; `TestSaveAssetRejectsSVGAndMarkup` documents that even a stored PNG-with-trailing-`<svg>` is served as `image/png`, never SVG.
- **AC2 (upload validation):** `TestSaveAssetRejectsSVGAndMarkup`, `TestSaveAssetRejectsNonImage`, `TestSaveAssetRejectsPixelBomb` (crafted 20000×20000 header rejected before decode, no file written), `TestSaveAssetRejectsOversizeBytes` (general + favicon), `TestSaveAssetFaviconRequiresPNG`, `TestSaveAssetAcceptsJPEGAndWEBPFallbackFormats`.
- **AC3 (fixed-enum atomic write / reset):** `TestSaveAssetHappyPathWritesFileAndSentinel` (fixed basename, no temp residue), `TestSaveAssetRejectsUnknownAssetKey` (incl. `../logo`), `TestResetAssetRemovesFileAndClearsSentinel` (idempotent + unknown-key rejected).
- **AC4 (presence sentinel / anon subset / CSRF / oneof):** `TestPublicEnhancementSettingIsStrictSubset` (four image presence sentinels public; watermark/watermarkShow forbidden; ⊆ authed), `TestSaveAssetLoginBackgroundSentinelSurvivesReadPath`; routes verified in the SessionAuth+PasswordExpired group behind the global `CSRFTokenGuard`; image keys absent from `EnhancementSettingUpdate.oneof`.
- **AC5:** full core `go test ./...` exit 0; adversarial security review recorded (below).

## Security review (passed)
Workflow `wf_cdaea91b-aad` — 7 independent security lenses (serve-XSS T1/T2, upload-decode T1/T5/T10, path/fs T3/T6, DoS T4, anon-leak T8, CSRF/authz T9, logic-correctness) each returned **no findings**; a completeness critic raised 7 hygiene notes, **all adversarially refuted as not-a-bug** (every vector neutralized by an existing control in the diff). Three notes were adopted as non-security quality polish (decode comment precision, serve/upload allowlist parity, frontend accept-list alignment). 0 confirmed exploitable defects.

## Human-needed (10-HUMAN-UAT.md)
Live curl of the anonymous endpoint (presence sentinels only, no bytes/paths); upload each asset type and confirm serve Content-Type + `nosniff`; confirm an SVG/oversize/pixel-bomb upload is rejected; confirm a stored polyglot is served as its raster type, not SVG.

## Note
Running the full core `go test ./...` regenerates `core/cmd/server/docs/x-log.json` and `agent/cmd/server/docs/x-log.json` (a `swagger_test.go` side-effect). Both were reverted; the release build uses focused/compile-only tests and does not regenerate them.
