# Phase 13 — Node Registry + CA/PKI + Enrollment Token (backend)

**Milestone:** v1.5 Secure Multi-Node (Slice A)
**Requirement:** NODE-PKI-01, NODE-REG-01
**Design:** `.planning/research/NODE-ENROLLMENT-DESIGN.md` (threats N1-N15)

## Why
The community source ships a hollow mTLS shell for remote nodes (two-mode listener, `RequireAndVerifyClientCert`, encrypted `ServerCrt/ServerKey/RootCrt` settings, `Proxy-Id` binding) but no CA issuance, no enrollment handshake, no node registry, and a `ValidateCertificate` that returns `true`. Core owns no CA and cannot sign a joining node. This phase builds the security core: a core-owned private CA, a single-use HMAC enrollment token, a CSR-signing enroll endpoint, and the `nodes` registry — all pure-Go and CI/loopback verifiable without a second host.

## Seam (reuse, do not fork)
- Router build-tag DI `core/router/entry.go` (`!xpack && !enterprise`) → `commonGroups()`; add `NodeRouter`.
- `PrivateGroup = /api/v2/core` so `Router.Group("nodes")` serves the frontend's existing `/core/nodes/list`, `/core/nodes/simple/all` calls (`frontend/src/api/modules/setting.ts:43-51`), shapes per `NodeItem`/`SimpleNodeItem` (`api/interface/setting.ts:311-334`).
- `CSRFTokenGuard` skips cookieless calls (`middleware/csrf_protect.go:59-60`); `CoreAPIAuthMiddleware` passes requests with no `1Panel-Token` header (`app/auth/api_auth.go`). So the machine-to-machine enroll endpoint needs no session and is safe without CSRF.
- `encrypt.StringEncrypt/Decrypt` (core `EncryptKey`) for private material at rest; `settingRepo.GetValueByKey/UpdateOrCreate` for singletons.

## Threats addressed here
N1 (single-use), N2 (HMAC forgery), N3 (TTL/nodeId scope), N4 (master-fp embed), N5/N6/N8 (fingerprint pinning in `nodepki`), N13 (token-gated CSR signing; CN imposed by core), N14 (SSRF addr validation).
