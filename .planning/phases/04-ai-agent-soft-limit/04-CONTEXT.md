# Phase 4: AI Agent Soft Limit - Context

**Gathered:** 2026-07-10
**Status:** Locked after implementation commit; live capacity acceptance remains open

<domain>
## Phase Boundary

This phase removes the community/license-derived hard count of five AI Agents and replaces it with the optional operator setting `AIAgentLimit`. Missing or zero means unlimited; a positive value is a soft count limit. The phase also bypasses App Store metadata limits only for AI Agent installation. It does not change license state, promise unlimited host resources, provide a dedicated settings UI, or make count-and-create enforcement transactional.

</domain>

<decisions>
## Implementation Decisions

### Limit semantics
- **D-01:** Read `AIAgentLimit` from the existing agent settings repository. Missing, blank, zero, invalid negative, or unparsable values behave as unlimited; invalid persisted values emit a warning.
- **D-02:** The agent settings update API accepts `AIAgentLimit` values from 0 through 1000. Zero is normalized and persisted as the unlimited value.
- **D-03:** A positive limit blocks creation when current count is greater than or equal to the configured value and uses the existing localized limit error with the configured maximum.

### License and App Store behavior
- **D-04:** Remove the enterprise/xpack branch from AI Agent count enforcement; never set or spoof a Pro/Enterprise state.
- **D-05:** Add `SkipAppLimit` to internal install hooks and set it only for AI Agent creation, so App Store metadata does not reintroduce a second count cap. Normal app installations still enforce their metadata limits.

### Compatibility and scope
- **D-06:** Preserve existing port, name, account, model, compose, application, and lifecycle validation.
- **D-07:** Update localized error text to refer to the configured limit rather than a fixed community value of five.
- **D-08:** Do not add a dedicated frontend control in this phase. Operators set the value through the existing agent settings API or equivalent administrative mechanism.

### Concurrency and acceptance boundary
- **D-09:** Count then create is intentionally a soft check and is not atomic. Concurrent requests can both observe capacity and exceed the configured limit.
- **D-10:** Automated tests and builds are complete. Creation of the sixth, tenth, or twenty-fifth live Agent, positive-limit blocking on a VPS, and resource-pressure behavior have not been tested.

### Agent Discretion
- A future phase may add a dedicated UI and transactional reservation without changing the current zero/unlimited contract.

</decisions>

<specifics>
## Specific Ideas

- "Unlimited" means no software count cap, not unlimited CPU, memory, disk, ports, Docker capacity, or external model quota.
- The App Store metadata bypass must be scoped to AI Agent installs and must not weaken ordinary app limits.
- Documentation must call the setting a soft limit because concurrent create operations are not serialized.

</specifics>

<canonical_refs>
## Canonical References

### Product and acceptance
- `.planning/PROJECT.md` - No license emulation and resource-conscious release boundary.
- `.planning/REQUIREMENTS.md` - AGENT-01 acceptance criteria.
- `.planning/ROADMAP.md` - Phase 4 goal and success criteria.

### Implementation record
- Commit `c305d759133ae7e22f90f11e16921f0e722f9bed` - Actual Phase 4 implementation and tests.
- `agent/app/service/agents.go` - Limit loading, capacity check, and AI install hook.
- `agent/app/service/app.go` - Scoped App Store metadata limit bypass.
- `agent/app/service/setting.go` - 0..1000 update validation.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Agent repository pagination provides the current persisted Agent count.
- Existing agent settings endpoints and repository provide key/value storage.
- Internal app installation hooks already scope AI-specific file preparation.

### Established Patterns
- Agent creation performs name/port/account/application validation before installation.
- App Store limit enforcement is centralized in `checkRequiredAndLimit`.
- Localized bus errors carry the maximum value to all supported languages.

### Integration Points
- `AgentService.Create` applies the optional soft limit and passes AI-only hooks.
- `AppService.installWithHooks` decides whether to enforce App metadata limits.
- `SettingService.Update` validates operator input.

</code_context>

<deferred>
## Deferred Ideas

- Dedicated frontend control for `AIAgentLimit`.
- Atomic reservation or database constraint for concurrent creation.
- VPS creation and resource tests beyond five Agents, including 10 and 25 where host capacity permits.

</deferred>

---
*Phase: 04-ai-agent-soft-limit*
*Context locked: 2026-07-10*
