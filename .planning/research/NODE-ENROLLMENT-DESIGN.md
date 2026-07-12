# Secure Multi-Node / Node-Enrollment Design + Threat Model

**Source-verified** 2026-07-12 against revision `d92047567` (branch `open-pro-v1`, upstream base `8be2a9ab0`, officialBaseTag v2.2.3). Triangulated by three independent source probes (node-registration map, agent-side auth/listen, core↔agent transport). Every claim below is anchored to `path:line`. This is the keystone the 15+ commercial capabilities depend on; it is a **security feature**, so the enrollment/PKI must be REAL — a no-op that flips a flag is explicitly forbidden by project doctrine.

## Naming caution (read first)
"Agent" is overloaded. **Host agent** = the `agent/` Go module (one process per host) — this is what multi-node federates. **AI agent** = `model.Agent`/`MasterAccountID` (the LLM copilot feature) — unrelated; ignore it. `core/app/model/agent.go` is a misnamed file that actually holds `WebsiteSSL` — ignore it too.

## The core finding: community ships a HOLLOW mTLS shell, gated off

The remote-node transport is **already scaffolded and sound in shape** in the community tree, but every piece that makes it a *security feature* is a closed-source stub. We supply exactly the missing pieces behind the existing build-tag seams — we do **not** build a parallel transport.

**Already wired (reuse, do not rebuild):**
- Two-mode agent listener — `agent/server/server.go:135`:
  - **Master mode** (`global.IsMaster==true`): Unix socket `/etc/1panel/agent.sock`, dir `0700`/file `0600` + owner-uid enforcement (`server.go:44-91,141`), **no TLS, no wire auth**.
  - **Node mode** (`!IsMaster`): binds `0.0.0.0:{Base.Port}` (`server.go:155`) over HTTPS with **`tls.RequireAndVerifyClientCert`** (`server.go:176`); server cert/key from encrypted settings `ServerCrt`/`ServerKey` (`server.go:157-172`); client CA pool from setting `RootCrt` (`server.go:178-184`); `ListenAndServeTLS("","")` (`server.go:188`).
- Socket routing / node selection — `core/init/router/proxy.go`: target from `?operateNode=` or header `CurrentNode` (`:28-34`); **auth runs in core before proxying** (`checkSession`/`API_AUTH`, `:41-48`); operator identity stamped as header `X-Panel-User` (`:50-52`); `local`/"" → `proxyLocalAgent` unix socket (`:54-62`), else → `xpack.MultiNodeProvider.Proxy(c, currentNode)` (`:63`).
- Local reverse proxy — `core/init/proxy/proxy.go:11,31,43` (`http://unix` over the socket); service-level `NewLocalClient` (`core/utils/req_helper/proxy_local/req_to_local.go:21-90`).
- Agent-side gate — `agent/middleware/certificate.go:17-40`: master short-circuits (`:19-22`); node path calls `xpack.MultiNodeProvider.ValidateCertificate(c)` (`:23`) then compares header `Proxy-Id` to file `/etc/1panel/.nodeProxyID` (`:32-37`); `CloseDirectly` RST-closes bad peers (`:42-55`). Mounted only when `!IsMaster` (`agent/init/router/router.go:20-24`).
- PKI storage — `RootCert` model + `InitDefaultCA` "1Panel-CA" (`agent/init/migration/migrations/init.go:81,230-248`); settings `EncryptKey`(16-char per-host), `ServerCrt/ServerKey/RootCrt/NodeScope/NodePort` (`init.go:97-124`); `NodeInfo` struct (`agent/app/model/setting.go:18-25`).
- Build-tag DI seams (`//go:build !xpack && !enterprise`) — `core/utils/xpack/community.go:1-9`, `agent/utils/xpack/community.go:1-9`. Interfaces: `core/utils/xpack/providers/multi_node.go:10-20`, `agent/utils/xpack/providers/multi_node.go:11-23`. Menu refs (referenced-but-absent pro pages): `/xpack/node/dashboard`, `/xpack/cluster` (`core/init/migration/helper/menu.go:130,135`).

**Missing (the closed pieces we clean-room):**
1. **Node registry** — no `nodes` table/model/service/API anywhere in the open tree (lives in closed `xpack.db`).
2. **CA issuance / cert generation** — `LoadNodeInfo` returns empty `ServerCrt/ServerKey` (`agent/utils/xpack/helper/multi_node.go:31-38`); nothing generates a CA, signs a leaf, or provisions `RootCrt`.
3. **Enrollment / token exchange** — nothing writes `RootCrt` or `/etc/1panel/.nodeProxyID`; no join token, CSR flow, or TOFU-free bootstrap.
4. **Core-side mTLS client transport** — `MultiNodeProvider.Proxy` is a local-only no-op (`core/utils/xpack/helper/multi_node_helper.go:21-33`); `LoadRequestTransport` is `InsecureSkipVerify:true` with **no client cert** (`:41-52`) — cannot even satisfy a node's mTLS.
5. **Real per-request cert identity check** — agent `ValidateCertificate` is `return true` (`agent/utils/xpack/helper/multi_node.go:65-67`).
6. **Rotation / expiry / revocation** — none in-tree.

## Trust model (the clean-room design)

- **Core host = the private CA + registry authority.** It owns the `RootCrt` CA keypair and the authoritative `nodes` table.
- **Node = a remote agent in node mode**, bound `0.0.0.0:{NodePort}` over mTLS.
- **Core → node** connects as a **TLS client presenting a client cert signed by the CA**; the node enforces `RequireAndVerifyClientCert` + `ClientCAs=RootCrt` (already wired). Core additionally **pins the node's server-cert fingerprint** from the registry (RootCAs=CA is not enough — a CA that signs many nodes would let node A impersonate node B; per-node fingerprint pinning closes that).
- **Proxy-Id** = a per-node random secret written to `/etc/1panel/.nodeProxyID` and stored in the registry; sent as header, binds an enrolled agent to exactly one master.
- **Identity over the wire:** `X-Panel-User` is trusted **only** when the mTLS peer authenticated as the bound master. A node must never honor `X-Panel-User` from an unauthenticated caller (today master mode trusts it blindly over the socket — acceptable same-host, unacceptable over a network).

## Enrollment handshake (the missing piece — CSR pattern, never ship private keys)

1. Admin on core clicks **Add Node** (addr + NodePort). Core mints a **single-use, short-TTL enrollment token** = `HMAC(coreSecret, {nodeId, nonce, exp, masterServerCertFingerprint})` + the payload. Token row persisted `pending`.
2. Token delivered to the node out-of-band (SSH bootstrap, or admin paste). Node calls core's **enrollment endpoint over TLS, pinning the master fingerprint carried in the token** (TOFU-free — no blind first-contact trust).
3. Node generates its **own** keypair, builds a CSR (CN=nodeId, SAN=addr), POSTs CSR + token. Core validates the token (unexpired, HMAC-valid, still `pending`), **atomically burns it**, signs a leaf server cert, returns `{signedServerCrt, RootCrt(CA PEM), proxyId, coreClientCertFingerprint}`.
4. Node stores `ServerCrt`(signed)/`ServerKey`(its own, never transmitted) encrypted, stores `RootCrt` for `ClientCAs`, writes `proxyId` to `/etc/1panel/.nodeProxyID`, flips to node mode, restarts into the mTLS listener.
5. Core records the node's server-cert fingerprint + status `online` in the registry. The private key never leaves the node; the token is spent.

## Threat model (N-series — must be honored as slices land)

| # | Vector | Sev | Required control |
|---|--------|-----|------------------|
| N1 | **Enrollment-token replay** | critical | Token single-use: burned in one atomic DB transaction on redemption (`UPDATE ... WHERE status='pending'` rowsAffected==1); short TTL (≤10 min); random nonce. A replayed/expired/already-spent token is rejected. |
| N2 | **Enrollment-token forgery** | critical | Token authenticated by `HMAC-SHA256(coreSecret, payload)`; constant-time compare; reject any bad MAC. `coreSecret` is a per-install random ≥32 bytes, stored encrypted, never sent to nodes. |
| N3 | Token exfiltration / broad blast radius | high | Token scoped to exactly one `nodeId` (cannot enroll a different node), single-use, ≤10-min TTL. Compromise window is one node for one short interval. |
| N4 | **MITM on enrollment (fake core)** | critical | Token embeds the master's server-cert fingerprint; the node pins it before sending its CSR. No trust-on-first-use, no `InsecureSkipVerify` on the enrollment call. |
| N5 | **Rogue node / cert-reuse across nodes** | high | Core-as-client pins each node's leaf **fingerprint** from the registry, not just CA-chain validity; `RootCAs=CA` alone is insufficient. A cert our CA signed for node A cannot pass as node B. |
| N6 | **Rogue master / node hijack** | high | Node binds to one master via `Proxy-Id` file (`certificate.go:32-37`) **and** a real `ValidateCertificate` that pins the master's client-cert fingerprint. A stranger holding any CA-signed client cert is rejected. |
| N7 | **Self-asserted `X-Panel-User` over the network** | high | Honor `X-Panel-User` only after the mTLS peer is authenticated as the bound master (client-cert + Proxy-Id). Never trust it from an unauthenticated caller. (Master/socket mode keeps same-host trust — unchanged.) |
| N8 | **`InsecureSkipVerify` / no client cert on core→node** | critical | The node transport MUST verify the node's server cert (RootCAs=CA + fingerprint pin) AND present core's client cert. Replace the `InsecureSkipVerify:true` stub (`multi_node_helper.go:41-52`) for the node path only; the loopback/community socket path is untouched. |
| N9 | Private key at rest | medium | `ServerKey`/CA key encrypted with the per-host `EncryptKey` (existing pattern, `init.go:100`); CA key file `0600`. Documented residual: DB+key-in-same-store means DB compromise = key compromise (an existing upstream limitation we inherit, not regress). Node private key generated on the node, never transmitted (CSR pattern). |
| N10 | Cert expiry / rotation / revocation | medium | Leaf validity bounded (e.g. 397 d); registry status `revoked` drops a node from the pin set immediately (core refuses to dial it and rejects its cert); provide a re-enroll/renew path. Full auto-rotation may be a later slice, but expiry + manual revoke ship in the first backend slice. |
| N11 | `0.0.0.0` exposure / DoS on the node port | medium | mTLS `RequireAndVerifyClientCert` rejects unauthenticated peers at the TLS layer (cheap); `CloseDirectly` RSTs bad certs (already present, `certificate.go:42-55`); optional source allowlist to the master IP. Node port is TLS-only — no plaintext listener. |
| N12 | Downgrade / plaintext fallback | high | The node path is TLS-only; on cert/pin failure core refuses to proxy — it must never silently fall back to plaintext or to the local socket for a *remote* node. |
| N13 | Unauthenticated CSR signing / enrollment-endpoint abuse | high | The signing endpoint requires a valid, unburned, unexpired token; it signs **only** the CN bound to that token's `nodeId`; rate-limited; audit-logged. No token → no signature. |
| N14 | **SSRF / addr injection via node address** | high | `addr` validated as host:port (no schemes, no credentials, no metadata endpoints like 169.254.169.254); core dials only registry-listed, enrolled nodes; the reverse proxy target is derived from the registry row, never from a client-supplied URL. |
| N15 | Weak agent→core loopback token (inherited) | low | Existing `X-Panel-Local-Token` is presence-checked only, loopback-scoped (`core/middleware/ip_limit.go:53-67`). Out of scope to fix here (loopback-only, not the node path), but do NOT extend this weak scheme to remote nodes. |

## Frontend surface (recon: switcher exists, pages + lifecycle + honest gate are missing)

The community frontend already ships the entire node-*targeting* plumbing — only destination pages and lifecycle management are absent, and the gate is a license flag we must not forge.

- **Present (reuse):** node switcher popover (`layout/components/Sidebar/components/Collapse.vue`, `changeNode()` sets `globalStore.currentNode`), node drawer (`.../node-drawer/index.vue`), reusable `<NodeSelect>` (`components/node-select/index.vue`), node utils (`utils/node.ts`). The **axios interceptor injects header `CurrentNode`** on every request (`api/index.ts:41-47`) from `getOperateNodeOverride() || globalStore.currentNode`; per-request override via `utils/operate-node.ts` + `composables/useOperateNodeContext.ts`; store state `currentNode:'local'`/`currentNodeAddr`/`masterAlias` (`store/modules/global.ts:75-78`).
- **Read endpoints the frontend ALREADY calls but core does NOT implement** (verified absent — only a demo-allowlist reference at `core/middleware/demo_handle.go:52`): `listNodeOptions(type)`→`POST /core/nodes/list`, `listAllSimpleNodes()`→`GET /core/nodes/simple/all` (`frontend/src/api/modules/setting.ts:43-51`). **Slice A must create these**, matching the `NodeItem`/`SimpleNodeItem` shapes in `frontend/src/api/interface/setting.ts:311-334`.
- **Absent (re-create in Slice B):** node lifecycle clients (create/delete/enroll), and the pages `NodeDashboard`/`SimpleNode`/`/xpack/cluster/*` — the community loads `@/xpack/**` and `@/enterprise/**` via `import.meta.glob` and silently falls back to `EmptyComponent`/404 (`extensions/routes.ts:6-8`, `extensions/optional.ts:4-7`, `router.ts:101-104`). `src/xpack/` and `src/enterprise/` do not exist.
- **⚠ Honest-gate constraint (doctrine):** the node UI is gated on `isXpackOrEE = (isEnterprise && isEnterpriseLicensed) || isMasterProductPro` (`store/modules/global.ts:101-103`; used at `Collapse.vue:173,253-255`, `home/index.vue:819`). We **must not forge license state**. Slice B re-gates the community node UI on a genuine signal (e.g. "≥1 enrolled node exists" / a community capability flag), NOT by faking `isProductPro`/license — exactly the posture the branding milestones used (build a real community form instead of unlocking the license-walled xpack editor).

## Phase decomposition (by blast radius + verifiability)

- **Slice A (this milestone, v1.5 — backend PKI + enrollment core; CI + loopback-verifiable, no 2nd VPS to BUILD):**
  - `nodes` registry table/model/migration + service + authenticated CRUD API (behind SessionAuth + CSRF, the existing enhancement/setting posture).
  - CA management: generate/load the `RootCrt` CA (reuse `RootCert` pattern), sign per-node leaf certs, compute fingerprints. All pure-Go, unit-testable (chain verifies, wrong-CA rejected, expiry enforced).
  - Enrollment token: issue + redeem (N1 single-use/atomic-burn, N2 HMAC, N3 nodeId-scope+TTL, N4 master-fingerprint embed). Fully unit-testable (replay/forgery/expiry/tamper all rejected).
  - Real `MultiNodeProvider.Proxy` + `LoadNodeInfo` (core): dial `https://addr:NodePort` with a CA-verifying, fingerprint-pinning, client-cert-bearing transport (N5/N8/N12/N14).
  - Real agent `ValidateCertificate` (N6): pin the master client-cert fingerprint + honor Proxy-Id.
  - **Verification without a 2nd VPS:** unit tests for all crypto/token paths **plus a single-box loopback integration test** — spin a node-mode agent on `127.0.0.1:{port}` with a real issued cert, enroll it, and proxy a request end-to-end over real mTLS. This proves the whole security path on one machine; cross-network live acceptance is the only thing deferred.
- **Slice B (next — node management UI + `operateNode` switcher):** community re-implementation of the `/xpack/node` pages (add/list/remove/enroll, node switcher that sets `CurrentNode`/`operateNode`). Depends on the frontend recon (in flight).
- **Slice C (later — cross-network live acceptance):** real 2nd VPS, real enrollment over the public internet, real proxy of a live operation. Human UAT; needs the second box.

## Deliberately out of scope (this milestone)
- **RBAC / multi-user** (`AuthProvider.CoreRBACMiddlewares` nil, `ResetSuperAdminUser` no-op) — a separate commercial domain; not required for single-admin multi-node.
- `Sync`, `AutoUpgradeWithMaster`, `PushSSLToNode`, `ProxyDocker`, `UpdateGroup`, `CheckBackupUsed` — node-federation conveniences layered on top; enroll+proxy first.
- Auto cert rotation (expiry + manual revoke ship now; scheduled rotation later).
- Fixing the inherited loopback `X-Panel-Local-Token` weakness (N15) — loopback-only, orthogonal.

## Open decisions carried into Slice A
- CA keypair location: reuse agent `RootCert`/`InitDefaultCA` vs a core-owned CA. **Lean: core owns the CA** (core is the registry authority and the party that signs joining nodes); the agent already reads `RootCrt` from its own settings, so the enrollment response seeds it. Confirm during implementation which module holds the signing key.
- Token transport for step 2: SSH-push bootstrap vs admin copy-paste. **Lean: support paste first** (no SSH dependency, fully testable), add SSH-push as convenience later.
- Whether Slice A flips `global.IsMaster` in a shipped binary or only in the loopback test harness. **Lean: keep production default master; node mode is opt-in via a provisioned node install**, so we never regress the single-host security posture.
- Fingerprint algorithm + pin storage column shape (SHA-256 of DER, hex) — settle in the migration.

---
*Design by KanameMadoka520, 2026-07-12. Triangulated recon (3 probes). Extends the v1.4 roadmap; the branding cluster (v1.2–v1.4) is feature-complete and this opens the multi-node keystone.*
