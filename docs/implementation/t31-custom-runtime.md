# T31 custom command runtime contract

Status: frozen contract for local implementation. Not implemented in this build; the
runtime stays disabled and the interface must show the reason. This document is the
build-ready specification for the remaining implementation.

## Profile identity

A profile is versioned and administrator-approved. It binds, immutably:

- `id`, `name`, `version` (increments on re-approval);
- `image_digest` (`sha256:` + 64 hex) of an operator-published container image;
- `executable` (absolute path inside the image, no shell metacharacters);
- `argv` (fixed template, JSON array of strings; no shell interpolation);
- `protocol_version` (integer, currently 1);
- declared input, event, output, cancel, exit and usage semantics;
- approval record (`approved_by`, `approved_at`, `approval_evidence`);
- revocation (`revoked_at`) and policy/run references.

A tenant cannot submit an arbitrary shell command or executable path. The runner executes
only the approved image and profile.

## Validation rules

- `image_digest` matches `^sha256:[a-f0-9]{64}$` and is pinned, never a tag.
- `executable` matches `^[A-Za-z0-9._/-]+$` and is absolute.
- `argv` is a bounded list of bounded strings; no shell is invoked and no string is
  interpolated.
- `max_wall_seconds` 1–3600; `max_output_bytes` 1–16 MiB; `max_turns` 1–128;
  `concurrency` 1–8.
- Creation is unapproved. Approval requires the owner/administrator role and non-empty
  evidence. Revocation is immediate and fences future effects.

## Protocol (version 1)

- Input: one JSON document on stdin.
- Events: newline-delimited JSON on stdout; each event has `type` and bounded payload.
- Output: a final `result` event with a typed outcome.
- Cancel: a native cancellation signal; only the runtime's confirmed termination counts
  as cancelled.
- Exit: nonzero exit is a failure. **Exit 0 never means a validated repair.**
- Usage: if the runtime reports usage it is recorded; otherwise usage is `unknown` and
  the reservation is held, never zeroed.
- Malformed events, unknown event types, oversized output and ambiguous termination leave
  the outcome `unknown` and require reconciliation.

## Execution

- Runs through the existing `sandbox.SandboxRuntime` with the profile image digest and no
  inherited environment; the control plane never passes provider keys or host auth.
- Wall-clock, output, turn and concurrency budgets are enforced before and during the run.
- The process group is terminated on cancel or timeout; detached descendants are handled
  by the sandbox lifecycle.
- Secrets are held only by the runner for the job and expire with it.

## Authority

- Dispatch revalidates current job/attempt fencing, policy, profile approval/version,
  image digest, workspace/SHA and the durable budget reservation before any effect.
- A revoked profile, changed image digest or expired approval blocks dispatch.
- Audit records creation, approval, revocation and every admitted run.

## Acceptance

A real local container test must cover input/output, malformed events, nonzero exit,
timeout, cancellation, revocation and secret isolation. The profile/run/policy/image
digest are immutable per run. Unknown or unverified entitlement, headless behaviour,
container topology or approval interception keeps the profile disabled with an actionable
reason.

## Remaining implementation slices

1. Migration `030_custom_profiles.sql` with RLS and approval/revocation columns.
2. `internal/customcmd` domain, validation, executor over `sandbox.SandboxRuntime` and
   protocol parser, with race tests for malformed/unknown/timeout/cancel paths.
3. HTTP create/list/approve/revoke under `/api/v1/orgs/{orgID}/custom-profiles` and
   OpenAPI schemas.
4. Runner wiring so an admitted job can select a profile, with durable budget reservation.
5. Connections GUI panel showing capability state, approval, digest and disabled reason.
