# WAF-Proper (Own-Engine) Design — Community Coraza Gateway

**Source-verified** 2026-07-13 against branch `open-pro-v1` (base v2.2.3), synthesizing five read-only recon probes (WAF remnants; website↔nginx wiring; commercial feature surface; engine/integration options; threat scoping). Every code claim is anchored to `path:line`. This is the design brief for the **WAF-proper milestone**, which the user has **explicitly opted into building** after it was deferred as XL in v1.6 (`.planning/v1.6-MILESTONE-DECISION.md:25`). Continues the project's phase numbering (v1.6 monitoring = Phases 16–18); the first WAF phase is **Phase 19**. Proposed milestone tag: **v1.7 WAF-Proper (own-engine)** — but see Open Decisions §7 (version label is not load-bearing).

---

## 1. Verdict recap — this is greenfield, not a shim to flip on

**DEFINITIVE: there is no real WAF engine anywhere in the 1Panel-X tree.** The recon is unambiguous:

- **Zero engine.** Case-insensitive grep of `agent/`, `core/`, `frontend/src/` for `coraza|modsecurity|modsec` returns **0 hits** (outside `.planning` docs). `find` for `*.lua` returns **0 files**. Neither `agent/go.mod` nor `core/go.mod` declares any WAF/coraza/modsec dependency. No `*waf*` file exists under `agent/app/service`; no waf hits in `agent/router`, `agent/app/api`, `agent/app/model`, or `core/router`.
- **The only in-tree WAF code is a filesystem contract into a *closed*, separately-installed app.** `createWafConfig` (`agent/app/service/website_utils.go:489`), `createAllWebsitesWAFConfig` (`:431`), `moveDefaultWafConfig` (`:406`), `delWafConfig` (`:603`) all compute `wafDataPath = <openresty install>/1pwaf/data` and **gate every call on `if !fileOp.Stat(wafDataPath) { return nil }`** — i.e. they silently **no-op** unless the closed OpenResty `1pwaf` app is already installed. They write `conf/sites.json` (`[]request.WafWebsite`) and **copy** default rule files *from the closed app's own `rules/` dir* (`:421-426`). `delWafConfig` is version-gated on OpenResty `>= 1.21.4.3-2-0` (`:608`). The domain-sync shim (`website_domain.go:42,131`) and the `syncIpGroup` cronjob (`cronjob_helper.go:134→270`) follow the same closed-dir pattern.
- **The only data model is a DTO, not a table.** `request.WafWebsite{ Key string; Domains []string; Host []string }` (`agent/app/dto/request/website.go:318`). There is **no GORM model, no migration, no table** for rules/ACL/IP-groups/attack-logs.
- **The UI and menu are dead-ends in community.** Menu ID 112 `xpack.waf.name` → `/xpack/waf/dashboard` under the closed `xpack` group (`core/init/migration/helper/menu.go:126,129`) resolves into the **absent `@/xpack` frontend module** (`frontend/src/extensions/xpack.ts` `import.meta.glob` returns null). No WAF view exists under `frontend/src/views`; the two touchpoints are a dead link `goWafIpGroup()→/xpack/waf/blackwhite` (`frontend/src/views/cronjob/cronjob/operate/index.vue:1152`) and an i18n tooltip. `frontend/src/lang/modules/en.ts:3117` is an explicit **commercial upsell** string. There is **no `WafProvider` seam** in the xpack build-tag stubs (`agent/utils/xpack/community.go`, `core/utils/xpack/community.go`) — unlike `AlertProvider`/`MultiNodeProvider`.

**Consequence (honesty red-line, W11).** A community WAF means **shipping our own engine** — engine + rules + per-site attack-log store + block page + a whole frontend. We must **NOT** reuse or surface the closed-app stubs (`createWafConfig`/`moveDefaultWafConfig`/`delWafConfig`) as a working WAF: they only light up when the operator has separately installed the closed OpenResty image, and presenting a non-inspecting passthrough as "protected" is a red-line violation. The existing honest gate (menu upsell, `en.ts:3117`) is correct and must be preserved until a **real** engine backs the feature. The per-site "protected" signal must be derived from **genuine engine health** (ruleset loaded + listener up + recent eval), and `isProductPro`/license must never be fabricated.

---

## 2. Engine + architecture decision

### Chosen: Architecture A (sidecar refinement) — embed OWASP Coraza + CRS v4 in a dedicated Go proxy

Ship a new small Go binary — **`coraza-gateway`** — that is:

- **`github.com/corazawaf/coraza/v3`** (OWASP Coraza) — an actively-maintained, ModSecurity/SecLang-compatible pure-Go WAF library that passes **100% of the CRS v4 test suite** and exposes `txhttp.WrapHandler(waf, http.Handler)` for drop-in `net/http` middleware;
- **`coraza-coreruleset`** — ships **OWASP CRS v4 as an `embed.FS`**, so the binary carries the ruleset with zero external files;
- fronting a stdlib **`net/http/httputil.ReverseProxy`** to the real origin.

**Topology:** the gateway runs as a **dedicated docker-compose sidecar app** on OpenResty's compose network. **nginx/OpenResty remains the public TLS terminator and ACME/cert manager** and `proxy_pass`es **cleartext** into the gateway bound to loopback; the gateway inspects, then forwards to the origin (W8-a). This is grounded in the runtime reality that **sites are served by OpenResty as an agent-managed compose container** (`constant.AppOpenresty`), reloaded/rebuilt via `opNginx(nginxInstall.ContainerName, NginxReload)`, `compose.DownAndUp(nginxInstall.GetComposePath())`, and `docker compose ... build` (`agent/app/service/nginx.go`). Because `proxy_pass 127.0.0.1:port` from OpenResty resolves **inside the container**, the WAF target must sit on OpenResty's compose network (sidecar) or via host-gateway — which is exactly the shape the closed `1pwaf` used, and a paved road the agent already owns.

**Why a separate sidecar process, not embedding Coraza in the agent binary:** embedding the data plane in the management plane couples them — an agent restart/upgrade would drop live site traffic, a WAF crash could take down the panel (and vice-versa), and the hostile-traffic surface would live inside the management binary. A sidecar isolates blast radius, gets its own lifecycle/cgroup limits, matches how 1Panel already runs everything sites depend on (OpenResty, DBs, the original 1pwaf) as managed compose apps, and makes **"not installed = WAF is plainly OFF"** honest with no forged pro/protected state.

### Rejected alternatives (grounded in the engine-integration probe)

| Option | Verdict | Why rejected |
|---|---|---|
| **B — Coraza-SPOA** | Non-starter | SPOP/SPOE is a **HAProxy-only** offload protocol; nginx/OpenResty has no SPOA client, and **1Panel ships zero HAProxy**. Adopting it means introducing HAProxy at the edge, breaking the per-site OpenResty model and every TLS/domain/cert/rewrite feature built on it. `coraza-spoa` is itself preview-grade. |
| **C — OpenResty+Lua / Coraza nginx module** | Deferred (future "native topology") | Two sub-variants, both worse now: (1) `lua-resty-waf` is **archived/unmaintained** — unacceptable for a security feature — and there is no mature `coraza-lua`; (2) Coraza's `libcoraza` dynamic nginx module is real but requires **cgo + rebuilding the OpenResty image against its exact ABI**, is the **least-mature** Coraza connector, and a bad module can wedge the edge for **all** sites (not per-site-opt-in-safe). Harder to ship incrementally and to verify in CI. Keep on the roadmap to evaluate **after** the Go-proxy slice proves the rule pipeline + UX. The `components/lua_block.go` (`access_by_lua_block` emit/parse) makes this topologically native if revisited. |
| **D — hand-rolled Go rule engine** | Reject outright | Directly violates the red-line against presenting a weak shim as a working feature: no CRS v4 coverage, near-certain evasion/bypass gaps, no upstream rule/threat-intel updates, no independent test suite. Coraza exists precisely so this is unnecessary. |

Coraza's stable, first-class deployment is **as-a-Go-library / http middleware** (and proxy-wasm for Envoy/Istio — nginx is *not* a proxy-wasm host). The mature, honest route on this stack is embedding the Go library in **our own proxy**, not an nginx module or SPOA.

### Licensing verdict — clean for GPL-3.0

`corazawaf/coraza/v3`, OWASP CRS v4 (`coreruleset/coreruleset`), and the `coraza-coreruleset` embed wrapper are **all Apache-2.0**. Apache-2.0 is **one-way compatible with GPL-3.0** (FSF): the combined/derivative work ships as GPL-3.0 while the Apache-2.0 components retain their license + `NOTICE`/attribution. Statically linking these Go libs into a GPL-3.0 binary is fine (vendor + keep `LICENSE`/`NOTICE`). The sidecar-as-separate-process shape is even cleaner (**mere aggregation**). No CLA/copyleft conflict. **Caveat:** Apache-2.0 is **incompatible with GPLv2-only** — 1Panel-X is GPL-3.0 so this is moot, but keep the license at **GPL-3.0** (do not downgrade to v2).

---

## 3. Integration points (exact, cited)

Where per-site traffic gets routed through the WAF, and where nginx config/reload happens:

**(a) Per-site server block — creation + storage.** `configDefaultNginx()` (`agent/app/service/website_utils.go:258`) is the single generator: it parses the embedded `website_default.conf`, sets listen/server_name/access_log/error_log + a type-specific root/proxy, sets `config.FilePath = GetSitePath(website, SiteConf)`, and writes via `nginx.WriteConfig`. Path map `GetSitePath` (`website_utils.go:1524`): `SiteConf = {WebSiteRootDir}/conf.d/{alias}.conf`, `StreamConf = {WebSiteRootDir}/stream.d/{alias}.conf`; `WebSiteRootDir` defaults to `{DataDir}/www` (`website_utils.go:1480`). These `conf.d/*.conf` files are the actual server blocks included by the container's main `nginx.conf`.

**(b) Per-site data dir → container mount.** `createWebsiteFolder()` (`website_utils.go:209`) builds `{WebSiteRootDir}/sites/{alias}/` (`log/`, `index/`, `ssl/`, `proxy/`, `rewrite/`, `redirect/`, `cache/`); inside the OpenResty container this is `/www/sites/{alias}/...` — every include/root/access_log string is hardcoded to that prefix (`website_utils.go:329-331`).

**(c) Mutate-in-place edit pattern (with rollback source).** All edits go `getNginxFull(website)` (`nginx_utils.go:23`, resolves the site file via `GetWebsiteConfigPath` `nginx_utils.go:54`, keeps `OldContent` for rollback) → `config.FindServers()[0]` → mutate via `Server` methods → `nginx.WriteConfig(config, IndentedStyle)` (`os.WriteFile(c.FilePath, DumpConfig(...))`, `dumper.go:126`).

**(d) KEY WAF-injection seam — the per-site proxy include.** `WebsiteService.OperateProxy` (`agent/app/service/website_proxy.go:26`) is the canonical extra-directive injector: it writes a **fragment** `sites/{alias}/proxy/{name}.conf` (a single `location ^~ /path { proxy_pass ...; proxy_set_header ...; }`) and persists the wiring by adding **`include /www/sites/{alias}/proxy/*.conf`** to the server scope via `updateNginxConfig(NginxScopeServer, ...)`. Enable/disable = rename `.conf`↔`.bak` (`UpdateProxyStatus:422`); delete = `DeleteProxy:402`. **This is exactly the shape to inject a WAF hop:** write one fragment `sites/{alias}/proxy/waf.conf` whose `location / { proxy_pass http://waf-gateway:PORT; proxy_set_header ...; }` sends traffic to the gateway — the `include` is already present, survives start/stop (`opWebsite()` re-adds it, `website_utils.go:1043`), and reuses the exact create/edit/enable/delete lifecycle.

**(e) Whole-site front alternative.** For a whole-site reverse-proxy front, reuse `constant.Proxy` type semantics — `createProxyFile` (`website_utils.go:176`) + `server.UpdateRootProxy` (`agent/utils/nginx/components/server.go:355`, builds `location / { proxy_set_header...; proxy_pass <proxy>; }`) — set the single root `proxy_pass` at the gateway upstream, which then forwards to the real backend. Directive primitives live in `server.go`: `UpdateDirective`/`RemoveDirective` (`:118/:159`), `UpdateRootProxy` (`:355`), `UpdatePHPProxy` (`:415`).

**(f) Config test + reload (always, with atomic rollback).** `opNginx(containerName, operate)` (`nginx_utils.go:241`): check = `docker exec -i {container} nginx -t`, reload = `docker exec -i {container} nginx -s reload` (20s timeout). `nginxCheckAndReload(oldContent, filePath, containerName)` (`nginx_utils.go:249`) runs `-t` then reload and, on **any** failure, writes `oldContent` back (atomic rollback). **Reuse this verbatim for the WAF fragment** so a bad WAF config auto-rolls back.

**(g) access_log source (for the attack-log/monitor later).** access_log is hardcoded to `/www/sites/{alias}/log/access.log` format `main` (`configDefaultNginx` `website_utils.go:330`; stream uses `streamlog` `:296`). Host-side path exposed as `res.AccessLogPath = GetSitePath(website, SiteAccessLog)` (`website.go:648`, `SiteAccessLog = {SiteDir}/log/access.log` `website_utils.go:1531`). Do not invent a new log path — derive WAF monitoring from this contract (the gateway also emits its own structured attack events; see Phase 20).

**(h) Site-creation / deletion lifecycle hooks.** In the create flow, the `configNginx` closure (`website.go:487`) runs `configDefaultNginx()` then `createWafConfig(website, domains)`; deletion calls `delWafConfig` + `delNginxConfig` (`website.go:745`). **New WAF provisioning slots alongside these** so it is created/torn down with the site and its domains (domains drive the `WafWebsite.Host` list). Key sites by `website.Alias` and enumerate `domain:port` hosts identically to stay compatible.

**Whole-site vs per-location choice:** the `proxy/*.conf` include (d) is the cleanest **per-location** reuse; `constant.Proxy` + `UpdateRootProxy` (e) is the cleanest **whole-site** reuse. Phase 22 scopes enablement **FIRST to reverse-proxy-type websites**, where inserting a WAF hop is natural with no static/PHP hairpin.

---

## 4. Parity target (from the feature-surface probe)

The original commercial WAF is a two-tier (global-default vs per-site) protection model. Reconstructed from the surviving i18n vocabulary (`frontend/src/lang/modules/en.ts` `waf:{}` block ~4739-5006 + `helper.wafTitle1-4`). Split honestly:

### MUST — the first credible slice (proves real enforcement)

The minimum vertical that reads as a genuine WAF, not a settings screen — a full loop of *rule edit → reload → request blocked → log row*:

- **M-a. Real Coraza + CRS v4 enforcement engine** (PL1 baseline) — SQLi/XSS/path-traversal and the CRS common groups actually block. This is the non-negotiable core (`sqliDefense`/`xssDefense` + CRS). **Honestly labelled:** live inline blocking on a real site is human UAT; the *engine* blocking fixture payloads is CI-verifiable.
- **M-b. Two-tier data spine: Global-default vs per-Website settings, master + per-site switch** (`globalSetting`/`websiteSetting`, `mainSwitch`, `saveToWebsite`, `globalSettingHelper`). This inheritance model is the architectural spine every rule type hangs off — **build first; retrofitting later is expensive.**
- **M-c. Per-site on/off toggle** that *really* flips whether nginx routes the site through the gateway; state reflects **true engine health** (W11), never a forged toggle.
- **M-d. Two modes:** detection-only (log, explicitly labeled fail-open) and block (fail-closed + block page). **New sites default to detection-only** so operators tune before enforcing (W1).
- **M-e. IP black/white lists** (`whiteList`/`blackList`, whitelist bypasses all restrictions) — the simplest credible enforcement beyond CRS.
- **M-f. Basic global-mode CC / access-frequency rate-limit** (`accessFrequencyLimit`, `ccHelper`: ">N req/same IP in Ns → block for Ms") — the signature WAF value-add. URL-mode / 404 / attack-frequency limiters are fast-follow, not MUST.
- **M-g. Minimal custom-rule (ACL) engine** — object (IP/UA/URL/header/args/cookie/method) + condition (`contain`/`equal`/`regex`/`notEqual`/`notContain`) + action (`deny`/`allow`/`blockIP`/return-code). The flagship differentiator; **challenge/captcha pages are LATER.**
- **M-h. Attack / block log** with source IP + hit rule + request detail (`blockRecords`, `attackLog`, `execRule`, `ruleType`). A WAF that can't show what it blocked is not credible — pairs with M-a..M-g to prove enforcement is real.
- **M-i. Static generic block page** (no origin leak, W7).

### LATER — XL parity (each gated on a real, testable capability; never UI ahead of engine)

- **Geo / region blocking + GeoIP DB + attack map** (`geoRule`, `reqMap` "Attack Map (Last 30 days)", `world`/`china`) — needs a GeoIP DB + map viz. Reuse `core/utils/geo` + `maxminddb-golang` (already agent-side) + the v1.6 ECharts monitoring components.
- **URL-mode CC / 404-frequency / attack-frequency limiters** (`uriMode`, `notFoundLimit`, `attackLimit`).
- **Full built-in group taxonomy** beyond SQLi/XSS: args/cookie/header defense, HTTP-method whitelist, file-extension blocklist, UA/bot rules, and the CVE/vuln corpus (`rce`/`ssrf`/`xxe`/`ssti`/`crlf`/`webShell`/...).
- **Custom-rule UI with author/edit + validate-before-load**, per-rule paranoia-level + false-positive tuning workflow (surfaces W10).
- **Challenge / JS / captcha interstitial** (`captcha`, `fiveSeconds`) — stateful challenge-token issuance runtime; significant.
- **Custom block/challenge pages** (`htmlRes`, `revertHtml`), **spider/bot pool** (`spiderIp`), **IP groups + remote sync** (partially present as the `syncIpGroup` cronjob shim — but that writes the *closed* dir today).
- **Attack report + dashboards / TOP-N home widgets** (`stat`, `wafSourceIpTop`, `wafAffectedSiteTop`, `wafIntercept`), **unknown-domain interception**, **CDN real-IP sharing**.
- **`libcoraza` native OpenResty module** (rejected Option C) — future "native topology" evaluation once the Go-proxy slice validates rules + UX.

---

## 5. Threat model / design controls (W1–W12)

The WAF's own attack surface. Several controls are **verbatim reuse** of already-shipped panel machinery.

| # | Threat | Mitigation |
|---|---|---|
| **W1** | **Fail-open vs fail-closed** on engine error/panic — a panic/eval error either crashes the proxy (DoS) or silently passes an uninspected request (bypass). | Wrap every per-request eval in `recover()` (reuse the `recover()`-that-swallows-only-`http.ErrAbortHandler` discipline at `core/utils/xpack/helper/multi_node_helper.go:68-72`) — but the WAF must then **decide block-vs-pass**, not just swallow. Policy: engine INIT / ruleset-load failure ⇒ **fail-CLOSED** (site reports engine-unavailable, no green state — ties W11); per-request eval panic ⇒ **fail-closed in block mode**, fail-open **only** in an explicitly-labeled detection/learning mode. **Never a silent fail-open in protect mode.** |
| **W2** | **Request smuggling / Host normalization** across client→nginx→gateway→origin (CL.TE/TE.CL desync, dup Content-Length, obs-fold, Host vs `:authority` mismatch). | Keep the Go `net/http` server as the parser (it rejects conflicting CL/TE + bare-CR framing); **disable HTTP/2 to upstream** (`ForceAttemptHTTP2:false`, mirroring `proxy.go:26`) for single-interpretation parsing; resolve the site policy from a **normalized Host matched against the site's registered `domain:port` set** (`WafWebsite.Host`, `website.go:318` / `website_utils.go:452` — the same set nginx uses via `proxy_set_header Host $host` `website_utils.go:202`); **default-deny unknown Host**. Inspect and forward the SAME normalized message; never forward ambiguous framing. |
| **W3** | **Resource exhaustion DoS** — body buffering (memory), slow-loris, pathological regex CPU. | Set Coraza `SecRequestBodyLimit` + `SecRequestBodyInMemoryLimit`, reject-oversize in block mode (bounded like weblog `maxLineLen`/`maxFieldLen` `parser.go:16-21`); the **public listener needs its own server-side `ReadHeaderTimeout`/`Read`/`Write`/`Idle` timeouts** — the current unix reverse-proxy sets only client-side dial/idle timeouts (`core/init/proxy/proxy.go:19-29`), none for an inbound listener; cap header count/size + max in-flight inspections; Coraza `@rx` uses **Go RE2 (linear, no catastrophic backtracking)** — keep transformations RE2-only; disable/limit response-body inspection. |
| **W4** | **Rule-bypass via encoding / multipart / charset** — URL/double-encoding, mixed case, null bytes, param pollution, multipart smuggling, utf-7/overlong. | Rely on CRS v4 + Coraza canonicalizing transforms (`t:urlDecodeUni`, `t:lowercase`, `t:removeNulls`, `t:compressWhitespace`) + Coraza's strict multipart parser (flag `REQBODY_ERROR` on boundary anomalies); **inspect the DECODED form but forward ORIGINAL bytes** so origin can't execute what the WAF never saw; reject/flag ambiguous Transfer-Encoding/charset; start at **PL1** (fewer FPs) with evasion transforms always on, PL configurable later (XL). |
| **W5** | **WAF admin/config API authz** — a separate WAF admin surface with weaker auth, or config-write reachable from the public data plane. | Register all WAF config/log routes under `agent/router` (like `ro_website.go`) so they inherit `SessionAuth` (`core/middleware/session.go:15`), the node `Certificate()` guard + `Proxy-Id` check (`agent/middleware/certificate.go:17-38`), and the **N7 unconditional `X-Panel-User` strip** (`core/init/router/proxy.go:54`). All mutations are **POST** (OperationLog audit + CSRF apply; GET skipped at `operation.go:39`). The management API binds **only** to the panel/agent socket — **never** to the public 80/443 WAF listener. |
| **W6** | **Attack-log injection** — stored attacker-controlled fields (matched URI, header/UA, payload snippet) inject CRLF/terminal-escape into logs, XSS into the viewer. | **Direct reuse of the shipped M1 control:** run every attacker-controlled field through the weblog `clean()` control-char strip + `maxFieldLen(2048)` cap (`agent/utils/weblog/parser.go:112-127`) before the store; write via parameterized gorm/`BaseModel` (the `website_stat.db`/monitor pattern); truncate payload snippets; the frontend log viewer renders as **text/interpolation, NEVER `v-html`** (same rule as M1, `WEBSITE-MONITORING-DESIGN.md:36`). |
| **W7** | **Origin IP / header leakage** — block pages, error responses, forwarded headers leaking internal IPs, server banners, `X-Powered-By`, stack traces. | The current `LocalAgentProxy` ErrorHandler writes `"Bad Gateway: "+err.Error()` (`core/init/proxy/proxy.go:49`) — for a **public** listener that leaks topology, so use the **generic-body ErrorHandler** (`WriteHeader` only, no body) from `multi_node_helper.go:62-64`. Block page = static templated page (reuse the `GetDefaultHtml` 404/default pattern `website_utils.go:167`) with zero upstream detail; strip hop-by-hop + sensitive upstream response headers; append client IP to `X-Forwarded-For` without reflecting internal topology; never forward the WAF's internal management headers to origin. |
| **W8** | **TLS termination placement** — WAF can only inspect plaintext; terminating TLS *after* it blinds it. | **Placement (a), recommended:** nginx stays the public TLS terminator + ACME/cert manager (reuse `website_ssl.go` + `opNginx` reload `website_ssl.go:600`) and `proxy_pass`es **cleartext** to the gateway on `127.0.0.1` — the WAF holds **no private keys** and re-implements **no ACME**. Placement (b) (WAF terminates TLS) duplicates the whole cert lifecycle → **rejected**. **Red-line corollary:** for a site configured as end-to-end TLS/stream passthrough the WAF **cannot** inspect and **must** report itself not-protecting — do not claim protection over an opaque stream. |
| **W9** | **Config-reload atomicity** — loading a malformed ruleset/policy takes the WAF down or leaves it half-loaded. | Mirror `nginxCheckAndReload`'s write→validate→rollback discipline (`agent/app/service/nginx_utils.go:249-259`). For Coraza: **compile the candidate ruleset into a fresh engine instance in memory FIRST**; only **atomically swap** the live engine pointer (`atomic.Value`/`RWMutex`) if it compiles clean; on failure keep the running engine and surface the error. **No live-editing of the in-use ruleset**; every write goes through the validated compile-then-swap path. |
| **W10** | **False-positive allow-listing safety** — operators fixing FPs create over-broad allow-lists (disable a rule globally, path `*`) that silently punch holes. | Allow-list entries are **SCOPED** (per-site + per-rule-id + per-path/param), stored as data, count-capped, linearly evaluated; every add/remove is a **POST** (audited via OperationLog); the UI honestly shows active suppressions + what each hides; allow-lists apply **only** through the authenticated config API (W5), never from attacker-influenced request input. **No "disable WAF for everything matching X"** without an explicit, logged, scoped entry. |
| **W11** | **RED-LINE — never forge license/pro state or ship an engine-absent shim.** Presenting a non-inspecting passthrough as "protected." | The closed 1pwaf/OpenResty engine is **entirely absent**; `createWafConfig`/`moveDefaultWafConfig`/`delWafConfig` (`website_utils.go:406-521`) only write JSON into a separately-installed image and must **NOT** be reused/surfaced as a working WAF. The WAF is a **REAL Coraza+CRS engine** compiled into the community binary or it is honestly **OFF**; the per-site "protected" signal derives from **genuine engine health** (ruleset loaded + loopback listener up + recent eval), exactly like v1.6's honest gate (`v1.6-MILESTONE-DECISION.md`); if a build lacks the engine the feature is **absent/greyed with a truthful "not built" message**, never a green shim; `isProductPro`/license is never fabricated. |
| **W12** | **Site-routing / policy-selection integrity** — an attacker forges Host to select a weaker/no WAF policy. | Resolve the applicable policy from the **authenticated site registry row** (the DB `Website`/`WafWebsite.Host` `domain:port` set), never from a raw client header alone — mirroring M3 (site lookup by DB row) and N14 (proxy target from the registry row only, `multi_node_helper.go:57`). Unknown/unmatched Host ⇒ **default-deny or strictest default**, never bypass. |

**Reused-verbatim controls:** (W6) weblog `clean()` + `maxFieldLen`; (W9) `nginxCheckAndReload` write→validate→rollback shape; (W5) `SessionAuth` + `Certificate()` guard + N7 `X-Panel-User` strip.

---

## 6. Phase plan (continues project numbering; v1.6 = Phases 16–18)

Doctrine, mirroring v1.6: **make the engine the CI-verifiable heart before any UI exists.** Each phase states its deliverable and whether it is **CI-verifiable** or needs a **live host/VPS (human UAT debt)**. The first WAF phase is the smallest honest, testable slice.

### Phase 19 — `coraza-gateway` engine + CRS v4 blocking core (the CI heart)
**Deliverable:** the standalone Go binary — `corazawaf/coraza/v3` + `coraza-coreruleset` (CRS v4 **PL1** embedded as `embed.FS`) + `txhttp.WrapHandler` over `httputil.ReverseProxy`. Detection-only vs block modes. Request decode/normalize wrapper (W4/W2). `recover()` block-vs-pass policy (W1). Compile-then-swap validated ruleset reload (W9). Server-side listener timeouts + `SecRequestBodyLimit` body caps (W3). Generic no-leak block page + `WriteHeader`-only ErrorHandler (W7).
**Verification — CI-verifiable, no nginx/live traffic:** fire known SQLi/XSS/path-traversal payloads at the wrapped handler and **assert HTTP 403**; clean requests **assert 200**; simulated eval panic ⇒ **block in block mode / labeled-pass in detection mode**; malformed candidate ruleset ⇒ **keep old engine**; oversize body ⇒ reject. **This is the smallest honest testable slice — it proves a REAL engine before a single line of UI.**
**Human UAT debt:** none yet (no site wiring).

### Phase 20 — Attack-event store + sanitized reader (backend)
**Deliverable:** the gateway emits structured attack events (source IP, matched rule id/type, normalized URI, sanitized payload snippet, action, code); a dedicated store mirroring the `website_stat.db`/host-monitor pattern (`BaseModel`, batch create, retention prune); **W6 `clean()` + field-cap sanitize** on every attacker-controlled field; parameterized writes; a read endpoint behind panel auth (W5).
**Verification — CI-verifiable:** sanitizer fixtures (CRLF/control-char/oversize → clean), store against a **temp sqlite**, retention prune, scoped-by-site read.
**Human UAT debt:** none (still no live routing).

### Phase 21 — Compose-app packaging + agent config generator (backend)
**Deliverable:** ship `coraza-gateway` as a **docker-compose app image** the agent installs/builds/reloads (reuse the `compose.build`/`DownAndUp` path in `nginx.go`) on OpenResty's network; agent config generator writes **per-site routing + per-site mode/paranoia/rule JSON** into the gateway's data dir via `files.FileOp`, mirroring the `createWafConfig`/`moveDefaultWafConfig` **filesystem-contract shape** (but into **our own** engine's dir, never the closed `1pwaf/data`).
**Verification — split:** config-generation + JSON contract are **unit-testable**; the actual container **build + run** needs a **live host (human UAT)**.

### Phase 22 — Per-site nginx wiring + honest enablement (backend)
**Deliverable:** scoped **FIRST to reverse-proxy-type websites**. Reuse the `proxy/*.conf` include seam (`website_proxy.go` `OperateProxy`) — write `sites/{alias}/proxy/waf.conf` → `proxy_pass http://waf-gateway:PORT` — with `nginxCheckAndReload` rollback (W9); a per-site on/off toggle that **really flips routing**; the "protected" signal **derived from genuine engine health** (W11); Host-normalized policy selection + **default-deny unknown Host** (W2/W12); hook create/delete into the site lifecycle (`website.go:487` `configNginx` closure / `:745` delete).
**Verification — split:** config-generation + rollback are **unit-testable**; **actual inline blocking on a live OpenResty site is human UAT** (documented as such — the only non-CI-reproducible piece, a thin adapter over the CI-tested engine).

### Phase 23 — Community WAF frontend + honest gate (frontend)
**Deliverable:** a website "WAF" tab — per-site toggle, mode selector, global-vs-site two-tier form (M-b spine), IP black/white lists, attack-log viewer (**text/interpolation, never `v-html`**, W6). **Honest gate:** if the gateway app is not installed / engine unhealthy, the tab shows **OFF/not-installed** — never a forged protected/pro state; **`isProductPro` never touched** (like the branding forms + node UI).
**Verification — split:** `npm run build:pro` + changed-file ESLint are **CI**; browser set/persist/render + honest-gate display are **human UAT**.

### LATER phases (24+) — XL parity, each gated on a real capability
Custom-rule ACL authoring UI with validate-before-load (extends W9); IP-groups + geo/country CC allow/deny + rate-based CC (reuse agent-side `core/utils/geo` + `maxminddb-golang`); challenge/JS/captcha interstitial (stateful token runtime); per-rule paranoia + FP-tuning workflow UI (surfaces W10); attack map + trend dashboards (reuse v1.6 ECharts + geo); anti-bypass hardening (HTTP/2, WebSocket passthrough policy, response-body inspection); and evaluation of the `libcoraza` native OpenResty module (rejected Option C) as a future native-topology path. **Each ships only when its underlying engine capability genuinely works — same honesty rule as Phase 19.**

**Milestone shape:** Phases 19–23 = the **first credible slice** (v1.7 WAF-Proper: real engine → store → packaging → wiring → UI). LATER phases = subsequent milestones (v1.8+).

---

## 7. Open decisions (need the user to choose)

1. **CRS distribution — bundle vs download.** Recommend **bundle** CRS v4 via `coraza-coreruleset` `embed.FS` (binary self-contained, CI-reproducible, no network at runtime, cleaner Apache-2.0 attribution). Alternative: download/update CRS out-of-band for fresher rules but adds a network dependency + supply-chain surface. **Decision needed:** bundle-only, or bundle + optional operator-triggered update?
2. **Default fail-open vs fail-closed.** Recommend **new sites default to detection-only (fail-open, explicitly labeled)** so operators tune before enforcing, with **block mode = fail-closed** (W1). **Decision needed:** confirm detection-only default, or default straight to block?
3. **Per-site vs global default scope of the two-tier spine.** The commercial model is global-default (new-site template) → per-site override (M-b). **Decision needed:** confirm we build the two-tier spine in Phase 19/22 (recommended — retrofitting is expensive), or ship per-site-only first and add global-default later?
4. **First-enablement website type.** Recommend scoping Phase 22 enablement to **reverse-proxy-type sites only** (natural WAF hop, no static/PHP hairpin), expanding to static/PHP/runtime types later. **Decision needed:** confirm reverse-proxy-first, or require all site types in the first slice?
5. **Whole-site front vs per-location include.** Recommend the **per-location `proxy/*.conf` include seam** (§3d) for opt-in safety and lifecycle reuse; the whole-site `UpdateRootProxy` front (§3e) centralizes at `location /`. **Decision needed:** confirm per-location include.
6. **`WafProvider` xpack seam.** Recommend adding a `WafProvider` to the xpack build-tag stubs (mirroring `AlertProvider`/`MultiNodeProvider`) so community hosts a real open WAF while keeping honest gating — rather than leaving `/xpack/waf/*` as dead upsell links. **Decision needed:** carve the seam, or implement directly in community code (as v1.6 monitoring did, no stub)?
7. **Milestone version label.** Proposed **v1.7**; not load-bearing. **Decision needed:** confirm or renumber.
8. **Body/response inspection limits + PL default.** Recommend **PL1** + request-body inspection with bounded limits, response-body inspection off by default (W3/W4). **Decision needed:** confirm PL1 start and response-body-off default.

---

*Design brief by KanameMadoka520, 2026-07-13. Synthesis of five read-only recon probes (WAF remnants; website↔nginx wiring; feature surface; engine/integration; threat scoping). Opens the WAF-proper (own-engine) milestone the user explicitly opted into: a REAL Coraza-Go + OWASP CRS v4 loopback reverse-proxy sidecar behind nginx TLS termination — engine-first (Phase 19 is the CI heart), honest-gated (W11), Apache-2.0-clean under GPL-3.0. No engine-absent shim, no forged pro state.*
