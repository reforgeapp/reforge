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

## Recipes

Go, JavaScript and Python validation recipes run in the sandbox. Recipe images are pinned
by digest and registered by an operator. Model usage is accounted before dispatch; an
unknown provider outcome holds the reservation instead of reporting zero cost.
