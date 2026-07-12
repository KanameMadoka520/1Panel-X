# Phase 14 — Real mTLS Proxy + Agent Cert Validation + Node Bootstrap (backend)

**Milestone:** v1.5 Secure Multi-Node (Slice A, part 2)
**Requirement:** NODE-PROXY-01, NODE-AGENT-01
**Design:** `.planning/research/NODE-ENROLLMENT-DESIGN.md` (threats N5/N6/N7/N8/N9/N12/N14)

## Why
Phase 13 built the core-side CA + registry + enrollment. This phase makes the two-way channel real: core dials nodes over pinned mTLS, and the agent stops trusting any CA-signed peer (`ValidateCertificate` was `return true`) and can be provisioned into node mode by an enrollment.

## What lands
**Core (`core/utils/xpack/helper/multi_node_helper.go`):** the remote branch of `Proxy` was a no-op (`c.Next()`); now it resolves the node from the registry, builds a per-node mTLS transport (core client cert + `RootCAs=CA` + exact server-cert fingerprint pin), and reverse-proxies to `https://addr:port` with the node's `Proxy-Id` header. Target derived only from the registry row (N14). `LoadNodeInfo` stays a no-op (SSH-based access is out of scope for the token+mTLS model).

**Agent (`agent/utils/xpack/helper/multi_node.go` + `enroll.go`, `agent/utils/nodepki/`):**
- `ValidateCertificate` now pins the master's client-cert fingerprint (N6) — the TLS layer already checks the CA chain; this binds to THE master. Fail-closed.
- `LoadNodeInfo` reads `NodeScope`/certs and enters node mode ONLY when `IsProvisionedNode(scope=="node" && serverCrt!="" && rootCrt!="")`. **Default stays master** (nil-DB safe at viper time; authoritative at hook time). Preserves the single-host unix-socket posture for every non-enrolled host.
- `nodepki` (agent-local, duplicated across modules by necessity): local key + CSR (private key never leaves the host, N9), token-claims read (to pin master, N4), fingerprint compare, enroll TLS pin.
- `Enroll`/`ApplyEnrollment`: node-join data plane — gen CSR, pin core, POST `/api/v2/core/nodes/enroll`, store certs+scope, write `/etc/1panel/.nodeProxyID`. Flip to node mode LAST (no half-provisioned node).

## Boot-order safety (verified)
`server/server.go`: `viper.Init()`(95, pre-DB) → `db.Init()`(99) → `hook.Init()`(115, `LoadNodeInfo(false)`, DB up, authoritative for `IsMaster`) → master/node branch(135). The viper-time call defaults master (nil DB); hook-time call determines the real mode.

## Deferred (Slice C, needs 2nd VPS)
Live cross-network enroll HTTP call + actual node-mode boot + real cross-network proxy. The CLI/UI trigger that invokes `Enroll` on a joining node is the operator entry point for that path.
