# Phase 13 Verification

**Status:** automated gates PASS; live cross-network acceptance deferred (Slice C, needs 2nd VPS).

## Automated evidence (WSL Go 1.26.1, authoritative Linux env)
- `gofmt -l` on all 10 new files → empty (clean).
- `go build ./...` (core module) → exit 0.
- `go vet ./utils/nodepki/... ./app/service/...` → clean.
- `go test ./utils/nodepki/ -count=1` → **ok**, 10 tests:
  - `TestCASignsChainableLeaf`, `TestSignLeafImposesCommonName` (N13 CN imposed), `TestSignLeafRejectsBadCSR`, `TestFingerprintStableAndDistinct`.
  - `TestTokenRoundTrip`, `TestTokenForgeryRejected` (N2), `TestTokenExpiredRejected` (N3), `TestTokenMalformed`, `TestIssueTokenRejectsShortSecret`.
  - `TestMutualTLSWithFingerprintPinning` — **single-box loopback mTLS proof**: real handshake + mutual fingerprint pinning succeeds; three rejection paths fire (N5 wrong server-fp pin → abort; N8 client cert from a foreign CA → RequireAndVerifyClientCert rejects; N6 wrong client-fp pin → abort).
- `go test ./app/service/ -run Node -count=1` → **ok**, 4 tests:
  - `TestValidateNodeAddr` (N14 — scheme/creds/path/embedded-control rejected), `TestValidateNodePort`.
  - `TestNodeEnrollSingleUse` — full DB enroll flow: create → enroll (returns serverCert/caCert/proxyID/coreClientFP, node → online, fingerprint pinned) → **replay of the same token rejected (N1)**.
  - `TestNodeEnrollRejectsForgedToken` — tampered token rejected (N2).

## What is NOT yet verified (carried)
- The core→node mTLS proxy and agent-side `ValidateCertificate` + node bootstrap land in Phase 14 (with a cross-module loopback proof).
- Live cross-network enrollment over the public internet — Slice C human UAT, needs a second VPS (unavailable). Not marked passed.
