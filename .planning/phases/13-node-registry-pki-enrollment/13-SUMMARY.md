# Phase 13 Summary

Delivered the backend security core for secure multi-node (Slice A, part 1).

## New files
- `core/utils/nodepki/nodepki.go` — dependency-free (stdlib crypto) PKI + enrollment-token core: `GenerateCA`/`LoadCA`, `SignLeaf` (CN imposed, CSR public-key only, EKU server/client, bounded validity), `GenerateKeyAndCSR`, `FingerprintDER/PEM`, `IssueToken`/`VerifyToken` (HMAC-SHA256, constant-time, TTL), `ClientTLSConfig`/`ServerTLSConfig` (mutual fingerprint pinning). `+ _test.go`.
- `core/app/model/node.go` — `Node` (secret fields json:"-").
- `core/app/dto/node.go` — `NodeInfo`/`SimpleNodeInfo` (match frontend), `NodeCreate`/`NodeSearch`, `NodeEnrollToken`, `NodeEnrollRequest`/`NodeEnrollResponse`.
- `core/app/repo/node.go` — `NodeRepo` + `BurnEnrollment` (atomic single-use, N1).
- `core/app/service/node.go` — `NodeService`: List/SimpleAll/Create(mint token)/Delete/Enroll(token-gated CSR sign); lazy `ensurePKI/initPKI` (CA + core client cert + HMAC secret, encrypted); `validateNodeAddr`(N14)/`validateNodePort`/SAN helpers; `coreServerFingerprint` (N4 best-effort). `+ _test.go`.
- `core/app/api/v2/node.go` — handlers.
- `core/router/ro_node.go` — authed group (list/simple/create/del under SessionAuth) + cookieless enroll group.
- `core/init/migration/migrations/node.go` — `AddNodeTable`.

## Wiring edits
`repo` entry (`service/entry.go`), `service` entry (`api/v2/entry.go`), `router/common.go` (register `NodeRouter`), `migrate.go` (register `AddNodeTable`).

## Deferred to later phases
Phase 14: core→node mTLS proxy + agent `ValidateCertificate` + node bootstrap + cross-module loopback. Phase 15: community node UI + honest re-gate. Slice C: cross-network live acceptance (needs 2nd VPS).
