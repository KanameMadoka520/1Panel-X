# Phase 15 — Community Node Management UI + Honest Re-gate (frontend)

**Milestone:** v1.5 Secure Multi-Node (Slice B)
**Requirement:** NODE-UI-01
**Design:** `.planning/research/NODE-ENROLLMENT-DESIGN.md` (frontend surface section)

## Why
The backend (Phases 13-14) can register/enroll/proxy nodes, but community has no UI to add/list/remove them. The existing top-bar node switcher is **license-gated** (`isXpackOrEE`) and **pro-coupled** (status vocab "Healthy", node roles, a `NodeDashboard` page absent in community). Doctrine forbids forging license state.

## Decision: dedicated community management page, not the pro switcher
Rather than fight the pro switcher's assumptions (or forge `isProductPro`), add a **community settings sub-page** that speaks the clean-room backend's own vocabulary. The settings sub-nav (`views/setting/index.vue`) is a hardcoded `buttons` array gated only by `isAdmin`/permissions — **no license check** — so the honest re-gate is simply an admin-only entry.

## What lands
- `routers/modules/setting.ts` — new static child route `nodes` → `@/views/setting/node/index.vue` (community, `adminOnly`).
- `views/setting/index.vue` — new sub-nav button `{ setting.nodes, /settings/nodes }`, gated on `isAdmin` (the honest signal; `isProductPro` stays false).
- `views/setting/node/index.vue` — list (reuses `listNodeOptions`), add-node dialog → shows the single-use **enrollment token** (copyable) + expiry, delete-with-confirm.
- `api/modules/setting.ts` + `api/interface/setting.ts` — `createNode`→`POST /core/nodes`, `deleteNode`→`POST /core/nodes/del`, `NodeCreate`/`NodeEnrollToken` types.
- i18n zh + en (`setting.node*`).

## Deliberately deferred (Slice C, needs 2nd VPS / operate flow)
- Re-gating the top-bar node switcher (operate flow) — pro-coupled; the operator switches `CurrentNode` to proxy to a node. The `CurrentNode` axios interceptor already exists.
- The node-side join trigger (CLI/UI that calls agent `Enroll`).
- Live add→enroll→switch→operate over a real network.
