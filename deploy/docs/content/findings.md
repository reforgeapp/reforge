# Findings

Findings are evidence-backed maintenance records discovered from bot pull or merge
requests, failing checks, advisories and deterministic checks.

## Triage

The **Findings** route lists source, category, severity, evidence age, repository, owner
and state. Open a finding for its evidence, related native links and current repair work.

Actions:

- **Queue repair** creates a bounded run (see [Runs and repair](repair.md)).
- **Assign** and **snooze with expiry** record the decision against the fingerprint.
- **Dismiss with reason** persists for that fingerprint and only reopens under documented
  changed-evidence rules.
- **Link existing work** records that a bot or human change already addresses it.

## Deduplication

Grouped or superseded updates share a canonical finding. Reforge does not generate a
duplicate update when a bot change already exists, and it recognises modifications made
to bot-managed branches. Where a bot has its own automerge authority, that conflict
blocks Reforge control rather than racing it.

Bulk actions preview eligible and skipped rows with the applicable cost ceiling before
anything is queued.
