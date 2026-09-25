# Repositories

The **Repositories** route is the scoped inventory. Rows carry forge, instance, team,
accessibility and last-sync state.

## Import

1. Open **Connections** and confirm the forge connection is healthy.
2. In **Repositories**, start an inventory sync, choose the connection and page through
   the eligible organisations, groups or owners.
3. Preview the matched selection. Import is asynchronous; large organisations import
   without blocking the interface.
4. Assign a team and, where the repository is private, a runner pool and approved private
   route. The probe runs from that runner, not from the control plane.

Unavailable or revoked repositories stay visible with their state and reason. A partial
sync never removes access to repositories it did not observe.

## Detail

Repository detail shows the detected stack and commands, effective policy, findings,
changes, deployment environments and branch-rule capabilities. Protection data carries a
timestamp and a refresh control; an unavailable rule is shown as unknown, never as
disabled.

## Scan and runners

**Start scan** on the repository Summary tab reads the default branch, dependency
manifests, open changes and check results. A failed scan names the access the forge token
lacks. **Settings → Runner pools** selects which active pools may run fixes for the
repository.

## Scope

Team and repository scoping is enforced server-side. Filters, sort and page state live in
the URL so deep links and browser back restore the list and selected row.
