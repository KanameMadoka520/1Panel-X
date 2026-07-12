# Phase 14 Verification

**Status:** automated gates PASS (both modules); live cross-network boot deferred (Slice C).

## Core module (WSL Go 1.26.1)
- `gofmt -l` on changed/new files → empty. `go build ./...` → 0. `go vet ./utils/xpack/... ./app/service/...` → clean.
- `go test ./utils/xpack/helper/` → **ok**:
  - `TestBuildNodeTransportPinsAndConnects` — loopback: core's pinned transport connects to an issued node server cert; **wrong pinned fingerprint aborts (N5)**.
  - `TestBuildNodeTransportRejectsUnenrolled` — no fingerprint → no transport.
  - `TestResolveNode` — resolve by addr / name / id; unknown rejected.
- `go test ./app/service/ -run Node` + `./utils/nodepki/` still **ok** (constant refactor caused no regression).

## Agent module (separate go.mod)
- `gofmt -l` clean; `go build ./...` → 0; `go vet ./utils/xpack/... ./utils/nodepki/...` → clean.
- `go test ./utils/nodepki/` → **ok**: `TestGenerateKeyAndCSR`, `TestParseTokenClaims`, `TestFingerprintEqual`, `TestEnrollTLSConfigPins` (master-fp pin match/mismatch).
- `go test ./utils/xpack/helper/` → **ok** (incl. pre-existing `alert_test` — no regression):
  - `TestIsProvisionedNode` — exhaustive: node mode ONLY on scope=node + both certs; **every other combination stays master**.
  - `TestApplyEnrollmentPersists` — stores encrypted ServerCrt/ServerKey/RootCrt (round-trips), lowercased master fingerprint, NodePort, NodeScope=node, writes ProxyID file (0600); incomplete response rejected.
  - `TestValidateCertificatePinsMaster` — matching peer accepted; **stranger cert rejected (N6)**; no-TLS rejected.

## Not verified (carried, Slice C — needs a 2nd VPS)
- Live `Enroll` HTTP handshake against a running core, the actual node-mode boot (`server.go` node branch), and a real cross-network browser→core→node proxy. The operator trigger (CLI/UI) that calls `Enroll` on a joining node.
- Single-host non-regression on a real box (community install still master, unix socket intact) — logically preserved by the `IsProvisionedNode` default + nil-DB guard; belt-and-suspenders live check is part of the deferred acceptance.
