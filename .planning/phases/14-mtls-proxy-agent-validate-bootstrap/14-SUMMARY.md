# Phase 14 Summary

Made the core↔node channel real and provisionable (Slice A, part 2). Both Go modules.

## Core
- `constant/node.go` (new) — node setting-key + status constants (shared by service + proxy helper; avoids a service↔xpack import cycle).
- `app/service/node.go` — refactored to the shared constants.
- `utils/xpack/helper/multi_node_helper.go` — real remote `Proxy`: `resolveNode` (addr/name/id) → `buildNodeTransport` (core client cert + CA + per-node fingerprint pin) → reverse-proxy to `https://addr:port` + `Proxy-Id` header. `+ _test.go` (loopback pin + resolve).

## Agent (separate module)
- `utils/nodepki/nodepki.go` (new) — node-side crypto: `GenerateKeyAndCSR` (key stays local), `ParseTokenClaims` (read master fp), `FingerprintDER/PEM`, `FingerprintEqual`, `EnrollTLSConfig` (pin core). `+ _test.go`.
- `utils/xpack/helper/multi_node.go` — `ValidateCertificate` pins the master (was `return true`); `LoadNodeInfo` enters node mode only when provisioned (`IsProvisionedNode`), default master, nil-DB safe.
- `utils/xpack/helper/enroll.go` (new) — `ApplyEnrollment` (persist certs+scope, write ProxyID, flip to node LAST) + `Enroll` (token→CSR→pin core→POST→apply). `+ _test.go`.

## Commits
7 scoped commits (core: refactor/feat/test; agent: feat/test/feat/test), all `KanameMadoka520`. 58 ahead of baseline; worktree clean.

## Deferred
Live cross-network enroll + node-mode boot + real proxy (Slice C, 2nd VPS). Operator join trigger (CLI/UI) that calls `Enroll`.
