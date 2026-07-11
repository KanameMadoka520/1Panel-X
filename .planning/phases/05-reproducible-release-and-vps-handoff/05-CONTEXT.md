# Phase 5: Reproducible Release and VPS Handoff - Context

**Gathered:** 2026-07-10
**Status:** Ready for planning
**Mode:** Auto-generated infrastructure context

<domain>
## Phase Boundary

Produce and inspect a reproducible Linux AMD64 release candidate from the committed v1.0 source revision, store release material under the external `image` directory, document VPS installation and rollback, write a timestamped Chinese roadmap note, and preserve every unperformed live acceptance check as explicit UAT debt.

</domain>

<decisions>
## Implementation Decisions

### Release Form
- Follow the upstream build model: compile native `1panel-core` and `1panel-agent` host binaries after embedding the production Vue build.
- Do not describe a privileged runtime container as ordinary isolation. The panel integrates with systemd, Docker, networking, storage, firewall, and host filesystems.
- The external `image` directory is the release-artifact boundary and may contain binaries, archives, checksums, metadata, logs, scripts, and VPS instructions.

### Reproducibility and Provenance
- Build only from a clean committed worktree and record the exact source revision, upstream merge base, toolchain versions, dependency-lock hashes, target, and checksums.
- Require Node 24.14.0, npm 11.14.1, and Go 1.26.1 without silently weakening version gates.
- Refuse proprietary `xpack` or `enterprise` overlay paths and do not emulate license state.

### Verification Boundary
- Run `npm ci`, the production frontend build, focused Go tests, package compile checks, and Linux AMD64 binary builds.
- Verify the top-level and in-archive checksums, ELF architecture, metadata, archive contents, and temporary-directory cleanup.
- Browser, real webhook provider, VPS restart, EICAR, live multi-Agent, and rollback acceptance remain pending until performed on an appropriate VPS.

### Documentation and Commits
- Keep source planning changes in scoped commits authored and committed by `KanameMadoka520 <2441883200@qq.com>`.
- Name the external roadmap note with a local timestamp plus a Chinese change summary.
- Describe this output as the first open-enhancement release candidate, not complete Pro or Enterprise parity.

### the agent's Discretion
- Exact release-note wording, artifact inspection commands, and the grouping of planning updates may follow repository and GSD conventions as long as evidence remains auditable.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `Makefile` already defines the native core and agent build entrypoints used by the release script.
- `D:/_CodeNotSync/_1Panel-X/image/build-release.sh` implements the clean-tree, toolchain, test, build, metadata, archive, and checksum pipeline.
- `D:/_CodeNotSync/_1Panel-X/image/README-VPS.md` contains backup, replacement, startup, smoke-test, feature acceptance, and rollback instructions.

### Established Patterns
- Frontend assets are produced by `npm run build:pro` and embedded under `core/cmd/server/web` before compiling the core binary.
- Linux verification uses WSL-native Node and Go toolchains, not Windows-native Go results.
- Human acceptance is persisted as `*-HUMAN-UAT.md` with `status: partial` so cross-phase audits cannot lose deferred work.

### Integration Points
- Release metadata links the external artifacts to the source Git revision and `upstream/dev-v2` merge base.
- Phase 5 summary and verification update `.planning/ROADMAP.md`, `.planning/REQUIREMENTS.md`, and `.planning/STATE.md` without closing UAT-dependent requirements.
- The external roadmap note links commits, checksums, artifact paths, automated results, limitations, and deferred VPS evidence.

</code_context>

<specifics>
## Specific Ideas

Use release version `v1.0.0-open.1` for the first candidate. Keep the package self-describing and suitable for upload to a disposable or fully snapshotted VPS.

</specifics>

<deferred>
## Deferred Ideas

- A standalone installer and upgrade service.
- A runtime OCI image claiming to replace native host integration.
- Full commercial feature parity beyond the four v1.0 enhancement domains.
- Completion of VPS, browser, provider, EICAR, capacity, and rollback UAT without an actual test environment.

</deferred>
