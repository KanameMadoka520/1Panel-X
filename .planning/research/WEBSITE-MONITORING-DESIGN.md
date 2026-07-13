# Website Access-Monitoring / Analytics Design (+ WAF deferral rationale)

**Source-verified** 2026-07-13 against branch `open-pro-v1` (base v2.2.3), by two independent Explore probes (website-monitoring surface; WAF engine presence). Every claim is anchored to `path:line`. This opens the "WAF / website monitoring" domain — but the two are very different in feasibility (below), so v1.6 delivers **website access-monitoring analytics** and **defers WAF-proper**.

## Why WAF-proper is deferred (engine absent)

The WAF engine (**1pwaf / OpenResty Lua**) is **entirely absent from the source tree** — zero `.lua`, no OpenResty config, no bundled engine. `1pwaf` appears only in 3 Go files as a **filesystem contract** writing JSON into a **closed, separately-installed OpenResty app image** (`agent/app/service/website_utils.go:406-654` `createWafConfig`/`moveDefaultWafConfig`/`delWafConfig`; `website_domain.go:42-140`; `cronjob_helper.go:270-303`). No WAF model/service/api/router/frontend exists; even the default rule schema lives inside the closed app. The `xpack.waf.*` i18n strings are consumed by the absent `@/xpack` frontend; `frontend/src/lang/modules/en.ts:3117` is an explicit upsell.

**Verdict:** a community WAF = **shipping our own engine** (embed Coraza-Go, or OpenResty+Lua) + rule/ACL/IP-group schema + per-site attack-log store/reader + block page + a whole frontend — **XL, multi-milestone**, and blocking behavior needs real traffic to verify. Per doctrine we will not present an engine-less shim as WAF parity. **WAF is a documented future milestone (opt-in), not part of v1.6.**

## Website monitoring: greenfield, but the value half of the WAF dashboard

Access-analytics (PV/UV/IP, QPS/traffic, request-log analysis, top-N ranking, visitor map) is **almost entirely absent** in community — only a raw nginx log viewer/tailer exists (`agent/app/service/website.go:1126 GetWebsiteLog` → raw text; UI `frontend/src/views/website/website/config/log/`). There is **no analytics xpack seam** carved out (unlike Alert/MultiNode) — so we implement it directly in community code (no stub needed), or optionally add a `WebsiteAnalyticsProvider` seam matching the house pattern. Nothing forces a stub.

### Existing scaffolding to reuse
- **Data source:** per-site nginx `/www/sites/{alias}/log/access.log` with the `main` log_format (`website_utils.go:330,1495,1524`; `cmd/server/nginx_conf/website_default.conf:7`; `constant/website.go:45`). `log_format main` is **not in the repo** — it ships with the OpenResty app; the parser assumes the standard nginx `main`/combined shape and is tolerant (skips lines it can't parse).
- **Time-series template:** the host-monitor subsystem — model `agent/app/model/monitor.go` (`MonitorBase` etc.), separate sqlite `global.MonitorDB` (`agent/init/db/db.go:14`, migrated `agent/init/hook/hook.go:177`), write/prune (`agent/app/service/monitor.go:323 CreateMonitorBase`, `:336 DelMonitorBase`, range query `WithByCreatedAt`). We mirror this with a `website_stat.db`.
- **GeoIP:** full MaxMind reader in `core/utils/geo/geo.go` (dep `oschwald/maxminddb-golang`); the dep + `1panel/geo/GeoIP.mmdb` are **already agent-side** (`agent/go.mod:35`, `agent/init/lang/lang.go`), only the agent-side reader is missing (copy it). Degrade gracefully when the DB is absent (as `core/app/service/logs.go:71`).

### Missing (must build)
Structured `main`-format parser; time-series + top-N store; incremental offset-tracked tailer + bucketed aggregator + retention prune; agent-side geo reader; UA parser (new dep) for device/browser ranking; API routes/service; frontend dashboard (charts + ranking + map) + route/menu.

## Architecture (v1.6)

- **Parser (pure, agent):** one access-log line → `AccessEntry{ts, ip, method, uri, status, bytes, referer, userAgent, xForwardedFor}`; regex over the nginx `main` shape; returns ok=false on malformed lines (never panics). **Unit-tested with fixture lines.**
- **Aggregator (pure, agent):** `[]AccessEntry` → per-(website, time-bucket) `WebsiteAccessStat{Pv, Uv(distinct IP in batch), Bytes, Status2xx/3xx/4xx/5xx}` + top-N `WebsiteAccessRank{kind(uri/ip/referer/status), key, count}`. **Unit-tested with fixtures** (assert PV/UV/status-class/top-N).
- **Store (agent):** dedicated `website_stat.db`, models `WebsiteAccessStat` + `WebsiteAccessRank`, `BaseModel`-based, batch create + retention prune (copy the monitor pattern). Tested against a temp sqlite.
- **Tailer (agent, thin I/O):** per-site offset-tracked incremental read of `access.log` on a schedule → parser → aggregator → store. The only non-CI-reproducible piece; a thin adapter over the tested parser.
- **API/service (agent):** `POST /websites/{id}/monitor/stat|rank` (range + kind) → aggregated read. Behind the existing site auth.
- **Frontend:** a website "Monitoring" tab/page — time-series charts (PV/UV/QPS/status) + ranking tables (top URI/IP/referer); ECharts (already used by host monitor). Honest gate: reachable in community for site admins, **no license forge**.

## Privacy / threat considerations (M-series for this domain)

| # | Concern | Control |
|---|---------|---------|
| M1 | Log-injection via forged `User-Agent`/`Referer`/URI (CRLF, control chars, huge fields) reaching the DB/JSON/UI | Parser bounds field length; store as data (parameterized SQL); frontend renders as text/interpolation, never `v-html`; strip/escape control chars. |
| M2 | Regex/ReDoS or unbounded memory on a malicious/huge access.log line | Line-length cap before parse; linear regex (no catastrophic backtracking); bounded batch size per flush. |
| M3 | Path traversal via `alias` when locating a site's access.log | Reuse the existing `GetSitePath`/site lookup (alias resolved from the DB Website row, never client-controlled free path). |
| M4 | PII (visitor IPs) at rest | Documented; retention prune bounds history; IPs are operational data the operator already has in access.log (no new exposure). No cross-site leakage (queries scoped by website id + site auth). |
| M5 | DoS via unbounded stat/rank growth | Retention prune (copy `DelMonitorBase`); top-N capped; bucket granularity bounded. |
| M6 | UA parser dependency risk (supply chain) | Pick a small, well-known pure-Go UA parser; pin version; parser failure degrades to "unknown", never crashes. |

## Phase decomposition

- **Phase 16 (v1.6, this milestone — backend core, CI-verifiable):** parser + aggregator + store + retention. Pure parser/aggregator unit-tested with fixtures; store against temp sqlite. **The heart; no live nginx needed.**
- **Phase 17 (backend I/O + API):** incremental tailer + agent geo reader (copy) + API/service endpoints. (UA/device ranking + GeoIP map may split to a follow-up if dep/fixture cost warrants.)
- **Phase 18 (frontend):** monitoring dashboard + honest gate + i18n.
- **Deferred:** WAF-proper (own-engine, XL); GeoIP visitor map + UA device ranking may be a v1.7 enhancement.

---
*Design by KanameMadoka520, 2026-07-13. Two source probes (monitoring surface + WAF engine presence). Opens the WAF/monitoring domain with the feasible, CI-verifiable monitoring half; WAF-proper deferred (engine absent).*
