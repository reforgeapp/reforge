# Runs and repair

A run is a bounded attempt to reproduce a failure, propose a change and validate it.

## Stage rail

Discover → Reproduce → Plan → Repair → Validate → Publish. After publication, change and
deployment status is tracked separately; the repair process does not stay alive while a
reviewer considers the change.

## What the run records

- the goal and the pinned starting revision;
- the intended file scope and the selected model route and billing route;
- the effective policy version at admission;
- the baseline failure and the candidate check results;
- the diff, artifacts and bounded logs.

Baseline and candidate checks use the trusted recipe. Deleting or disabling a test, or
rewriting its script to a no-op, cannot manufacture a passing result. A failed check
never publishes.

## Controls

Cancel terminates the sandbox process group and explains that already-published provider
actions remain visible. Resume is only available for a blocked run and re-checks current
authority. Reconcile resolves an unknown provider outcome before any retry; the same
request identity is reused so a lost response cannot create a second change.
On a stopped run, Reconcile also settles each model call with an unknown outcome at its
reserved maximum and frees the finding for a new run. A call the provider refused outright
is recorded as failed with no spend.

## Recipes

Go, JavaScript and Python validation recipes run in the sandbox. Recipe images are pinned
by digest and registered by an operator. Model usage is accounted before dispatch; an
unknown provider outcome holds the reservation instead of reporting zero cost.

The sandbox has no network. For Go, the runner downloads modules listed in `go.mod` and
`go.sum` from the public Go proxy before the run, without executing repository code, and
mounts them read-only. Private modules are not fetched. JavaScript and Python recipes use
only `node --test` and `unittest` and receive no third-party packages. Every recipe image includes `git` and `/bin/sh` for tests that create local repositories.

## CI-only failures

When the recipe's checks pass but the forge's CI failed (for example a dependency scanner),
the run fetches the tail of up to three failed CI job logs, redacted of tokens and keys, and
works on the target branch. The model can:

- update a dependency; npm or `go get` regenerates the manifest and lockfile inside the
  sandbox, with install scripts disabled, reaching only the public npm and Go registries
  through the runner's egress proxy;
- patch source files;
- edit existing CI workflow files, for example a pinned toolchain version;
- skip, when an open Reforge fix already covers the failure or it cannot be fixed from the
  repository (missing secret, provider permissions). Skipped findings are not retried.

When CI fails on a Reforge pull request, the next scan turns it into a follow-up: the run
works on the pull request's own branch and pushes a new commit to it. After five follow-ups
that still fail, Reforge closes the pull request with a comment naming the failing checks.

A Reforge pull request counts as fixed, and autopilot merges it, only when every check that
passes on the target branch also runs and passes on the pull request. A check that
disappears, for example because a workflow edit removed it, is treated as a failure and
followed up.

Tests stay protected. Workflow edits on GitHub need a token with `workflow` scope, or the
GitHub App's Workflows permission. The recipe's checks must pass before publication,
and the forge's required checks still gate the merge.
