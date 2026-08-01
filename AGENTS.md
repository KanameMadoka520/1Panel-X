# 1Panel-X Contributor Instructions

## Purpose

1Panel-X is an independent GPL-3.0-derived project built on the official 1Panel community repository. The long-term goal is an auditable open panel with independently implemented equivalents for useful publicly described enhancements. Work must preserve upstream history, remain reviewable, and never depend on proprietary source or license bypasses.

Read these files before changing code:

1. `.planning/PROJECT.md`
2. `.planning/REQUIREMENTS.md`
3. `.planning/ROADMAP.md`
4. `.planning/STATE.md`
5. Relevant files under `.planning/research/`

## Repository and Workspace Layout

```text
D:\_CodeNotSync\_1Panel-X\
|-- source\       Git repository and planning documents
|-- image\        Release packages, binaries, checksums, build material, VPS guide
`-- roadmap\      Timestamped material-update notes

source\
|-- core\         Go control plane, API, settings, authentication, embedded frontend
|-- agent\        Go execution plane, Docker and host operations, cron, ClamAV, AI agents
|-- frontend\     Vue 3, Pinia, Element Plus, and Vite frontend
|-- docs\         Upstream documentation
`-- .planning\    Project scope, requirements, roadmap, state, research, and phase artifacts
```

`upstream` points to the official repository and is for fetching and comparison. Do not push 1Panel-X commits to `upstream`, rewrite official history, or force-push shared work.

## Public Repository Policy

The public `origin` repository is a source-code storage and collaboration remote, not a CI/CD, deployment, mirroring, scheduled-maintenance, or automated-review platform.

- Keep `main` as the only local development branch and the only branch on `origin` unless the user explicitly authorizes another branch or pull request.
- Do not add or restore `.github/workflows`, GitHub Actions, Dependabot configuration, automated pull requests, repository mirroring, bot translation, automated code review, scheduled jobs, Pages deployment, or similar hosted automation without the user's explicit approval.
- Keep repository-level GitHub Actions disabled. If an upstream synchronization reintroduces hosted-automation files, exclude those files before committing the synchronization.
- Do not carry upstream Issue or pull-request templates into 1Panel-X by default. They can misdirect users to upstream maintainers or upstream support channels.
- Continue to run required tests and release checks locally or in an explicitly approved external environment, and record the results in the handoff. Do not replace verification with a hosted workflow.
- GitHub secret scanning and push protection may remain enabled because they do not add repository workflows, create development branches, or replace local verification.

## Legal and Product Boundary

- Keep distributed modifications GPL-3.0-compatible and preserve required notices and source availability.
- Use only public GPL code, public documentation, generated public API or operation descriptions, open standards, and lawfully observable behavior.
- Do not obtain, copy, decompile, reconstruct from disassembly, or redistribute proprietary `xpack` or `enterprise` implementations.
- Do not generate licenses, bypass activation, patch binaries, forge bound or Pro status, or globally force `isProductPro=true`.
- Do not present empty pages, fake APIs, mock data, or no-op handlers as completed functionality.
- Do not claim this project is an official licensed Pro build or use vendor branding in a misleading way.
- Treat WAF, node control, RBAC, anti-tamper, backups, webhook delivery, ClamAV, database HA, and VMs as separate security domains with explicit failure tests.

## Current v1.0 Scope

Only these deliverables belong to the Open Enhancement First Release:

1. Open theme color and authenticated watermark settings.
2. WeCom, DingTalk, and Feishu robot webhook alerts.
3. Durable ClamAV schedules with restart recovery and scan serialization.
4. AI Agent default unlimited count plus an optional operator soft limit.
5. Reproducible Linux AMD64 release artifacts and VPS instructions.

Advanced WAF, website monitoring, multi-node, synchronization, RBAC, anti-tamper, reports, database HA, AI gateway, VMs, mobile clients, AI site building, and SMS are future work. Do not imply they are included in v1.0.

## Change Discipline

- Inspect the current worktree before editing. Existing unrelated changes belong to another contributor and must not be reverted or reformatted.
- Prefer existing routers, services, repositories, models, extension fallbacks, and UI components over new frameworks.
- Keep changes within the owning module. Avoid import cycles such as a service importing a helper that imports the service again.
- Validate both incoming settings and already persisted values. Corrupt database state must fail closed or fall back to documented safe defaults.
- Never log credentials, webhook URLs, tokens, license data, private keys, or unredacted platform responses containing secrets.
- Use `apply_patch` for manual edits. Run `gofmt` on every changed Go file.
- Add focused tests with each behavior change. Security-sensitive code requires negative cases, not only success cases.
- A feature is not complete until tests pass, relevant UI or VPS checks pass, and the change has a scoped commit.

## Security-Specific Rules

### Webhooks

- Permit HTTPS only and restrict each provider to its documented official robot hosts.
- Verify TLS even if another application transport was configured insecurely; clone shared transports before changing them.
- Refuse redirects, bound timeouts and response sizes, validate HTTP and business codes, and redact complete secret URLs from errors and logs.
- Count failed sends as delivery attempts so one monitoring cycle cannot retry forever.

### ClamAV

- Persist a new rule before registering a callback that depends on its database ID.
- Validate and register an updated schedule before removing the old schedule.
- Restore enabled non-empty schedules after agent restart and update in-memory entry IDs.
- Serialize manual and scheduled runs for the same rule.
- Normalize and constrain scan, isolation, and deletion paths. Reject roots, traversal, symlink escapes, and isolation directories inside scan trees.
- Use restrictive isolation permissions. Run EICAR or `remove` tests only in a disposable directory, never against a real site or production path.

### AI Agent Limits

- Missing or zero `AIAgentLimit` means unlimited count, not unlimited host resources.
- A positive limit is an operator soft limit and must not alter or emulate product license state.
- Preserve port, name, application, Docker, and lifecycle validation. Document that count-then-create is not atomic unless later made transactional.

## Verification Environment

The Windows host does not provide an authoritative native Go environment for this Linux panel. Linux-only syscall code can fail or behave differently on Windows. Run Go formatting, tests, and release builds in WSL or Linux with the repository-compatible Go 1.26.1 toolchain.

Recommended WSL environment:

```bash
cd /mnt/d/_CodeNotSync/_1Panel-X/source
export PATH=/tmp/codex-go1.26.1/go/bin:$PATH
export GOPROXY=https://goproxy.cn,direct
export GOSUMDB=sum.golang.google.cn
export GOTOOLCHAIN=local
go version
```

Focused Go checks for the current milestone:

```bash
cd /mnt/d/_CodeNotSync/_1Panel-X/source/core
go test ./app/service -count=1

cd /mnt/d/_CodeNotSync/_1Panel-X/source/agent
go test ./app/service -count=1
go test ./utils/webhook_alert -count=1
go test ./utils/xpack/helper -run '^$' -count=1
```

Run broader package tests or compile checks before release. Record any environmental skip or upstream failure rather than hiding it.

Frontend verification:

```bash
cd /mnt/d/_CodeNotSync/_1Panel-X/source/frontend
npm ci
npm run build:pro
```

The repository currently has known upstream `npm run type-check` failures. Compare against the upstream baseline, fix regressions introduced by the change, and report inherited failures explicitly.

Linux AMD64 release build order:

```bash
cd /mnt/d/_CodeNotSync/_1Panel-X/source/frontend
npm ci
npm run build:pro

mkdir -p ../build
cd ../core
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o ../build/1panel-core ./cmd/server/main.go

cd ../agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o ../build/1panel-agent ./cmd/server/main.go
```

The panel is deeply integrated with the host, Docker, networking, storage, firewall rules, and system services. A container may be used as a reproducible builder, but do not describe a privileged runtime container as meaningful default isolation.

## Commit Rules

Configure the repository-local identity before every commit session:

```bash
git config --local user.name "KanameMadoka520"
git config --local user.email "2441883200@qq.com"
```

Required identity for both author and committer:

```text
KanameMadoka520 <2441883200@qq.com>
```

Commit by coherent delivery unit. For v1.0, keep theme and watermark, AI Agent limit, webhook alerts, ClamAV scheduling, and release documentation in separate commits. Stage explicit paths, inspect `git diff --cached`, and never sweep unrelated changes into a commit.

Verify every new commit:

```bash
git show -s --format='Author: %an <%ae>%nCommitter: %cn <%ce>%nSubject: %s' HEAD
```

Do not amend upstream commits. Do not commit or push unless the active task explicitly authorizes it.

## External Release and Roadmap Rules

Do not put generated release binaries in `source` unless a tracked build script or manifest explicitly requires it.

`D:\_CodeNotSync\_1Panel-X\image` is the release handoff directory. Each release should include:

- Native Linux binaries and a versioned archive.
- `SHA256SUMS` or an equivalent checksum manifest.
- Reproducible build commands or scripts and exact source revision metadata.
- `README-VPS.md` with prerequisites, backup, install or replacement, startup, rollback, smoke tests, and limitations.
- A concise record of automated checks and VPS acceptance evidence.

`D:\_CodeNotSync\_1Panel-X\roadmap` contains one Markdown note per material update. Use this filename pattern:

```text
<YYYYMMDD-HHMMSS>-<Chinese-change-summary>.md
```

The note should identify source commits, delivered behavior, tests, artifacts, deployment instructions, known limitations, and future work. Create the note only when a material update is being delivered; do not create placeholder roadmap files.

## Definition of Done

A task is done only when its scoped behavior exists, focused tests pass in Linux or WSL, relevant frontend or VPS checks are recorded, security failure paths are covered, documentation matches reality, and the requested commit identity and separation are verified. Anything less remains in progress and must not be described as full Pro or enterprise parity.
