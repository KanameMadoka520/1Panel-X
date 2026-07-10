# 1Panel-X

## What This Is

1Panel-X is a GPL-3.0 open-source server management panel derived from the official 1Panel community repository. It preserves upstream history and incrementally adds independently implemented, openly documented alternatives to capabilities that are publicly described as commercial or enterprise features.

The project is intended for self-hosters and infrastructure operators who need a reproducible, auditable panel build without depending on closed modules. Work is delivered through small, reviewable commits, timestamped Chinese roadmap notes, and VPS-oriented build and test instructions.

## Core Value

Deliver a complete, security-conscious, fully open server panel whose enhanced capabilities can be built, inspected, deployed, and maintained without proprietary code or license bypasses.

## Requirements

### Validated

- The official `1Panel-dev/1Panel` repository is present with full Git history and tracks `upstream/dev-v2`.
- The development branch is `open-pro-v1`.
- New commits use `KanameMadoka520 <2441883200@qq.com>` as author and committer.
- The workspace separates source, image/build artifacts, and timestamped roadmap notes.

### Active

- [ ] Preserve compatibility with the current upstream Go and Vue architecture.
- [ ] Implement enhanced features from public behavior and documentation without copying closed-source code.
- [ ] Deliver a first usable milestone with scheduled ClamAV scans, open webhook alert delivery, and license-independent theme/watermark customization.
- [ ] Add focused automated tests for new security-sensitive and scheduling behavior.
- [ ] Provide a reproducible Linux container/image build path and VPS test instructions under `D:\_CodeNotSync\_1Panel-X\image`.
- [ ] Record each material update under `D:\_CodeNotSync\_1Panel-X\roadmap` using a timestamp plus Chinese summary filename.
- [ ] Maintain a phased roadmap for multi-node management, WAF, anti-tamper, backup/monitoring, branding, RBAC, reports, and other publicly documented enhanced features.

### Out of Scope

- License key generation, activation bypasses, binary patching, or pretending a community build is an officially licensed Pro build - these are not required to create equivalent open functionality and introduce legal and security risk.
- Copying, decompiling, or redistributing proprietary 1Panel modules - implementations must be original and based on public interfaces, observable behavior, and open standards.
- Claiming feature parity before a capability has implementation, automated verification, and VPS-level acceptance evidence.
- Depending on undocumented 1Panel commercial cloud services when a local or open protocol can provide the same user outcome.

## Context

- Upstream is a brownfield Go + Vue application split into `core`, `agent`, and `frontend` modules.
- The source tree exposes build-tagged `xpack`/`enterprise` extension points while excluding proprietary implementations from Git.
- Several community helpers intentionally return no-op results or `ErrXpackNotFound`; these are high-value boundaries for clean-room open implementations.
- The host has Node.js, npm, pnpm, Git, and Docker, but no native Go installation. Verification should therefore prefer Dockerized Go toolchains and the repository's existing frontend toolchain.
- The first milestone is deliberately narrower than total commercial parity. Its purpose is to establish a tested extension pattern, prove deployability, and leave a trustworthy development chain for subsequent phases.

## Constraints

- **License**: All distributed modifications remain GPL-3.0-compatible and source-available.
- **Clean-room implementation**: Use public documentation, public APIs, open standards, and existing GPL interfaces only.
- **Upstream compatibility**: Keep changes localized and avoid rewriting official history so future `upstream/dev-v2` merges remain practical.
- **Security**: Treat WAF, anti-tamper, authentication, alert delivery, backups, and node control as security-sensitive; ship them only with validation and explicit failure behavior.
- **Build environment**: Native Go is unavailable on the Windows host; Linux builds and Go tests must be reproducible through Docker until a matching Go toolchain is installed.
- **Delivery**: Use multiple scoped commits and preserve the requested author identity on every new commit.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Track official `dev-v2` on a dedicated `open-pro-v1` branch | Keeps upstream updates reviewable and avoids altering official commits | Good |
| Implement outcomes instead of license emulation | Functionality is the product goal; license bypassing adds no durable capability | Pending |
| Start with existing public extension gaps | Public interfaces, UI, and data models reduce guesswork and integration risk | Pending |
| Use standard open protocols for alerts and local scheduling | Avoids proprietary service dependencies and makes behavior testable | Pending |
| Keep long-term parity in a phased roadmap | Multi-node, WAF, anti-tamper, and enterprise RBAC are separate security domains and cannot be responsibly treated as one patch | Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition**:
1. Move shipped and verified requirements to Validated with a phase reference.
2. Move invalidated requirements to Out of Scope with a reason.
3. Add newly discovered requirements to Active.
4. Record architecture and product decisions in Key Decisions.
5. Re-check that What This Is and Core Value still describe the project.

**After each milestone**:
1. Review all requirements and evidence.
2. Re-check the core value against delivered behavior.
3. Audit deferred and excluded scope.
4. Update build, VPS test, and maintenance context.

---
*Last updated: 2026-07-10 after project initialization*
