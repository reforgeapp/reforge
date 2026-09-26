# Policies

Policy controls automatic actions. Policies are immutable versions scoped to the
organisation, team or repository.

## Mode

**Mode** sets the organisation policy in one step. It saves, simulates and activates a new
organisation version.

| Mode | Permits |
| --- | --- |
| Observe | Read only |
| Propose | Repair and publish changes |
| Merge | Also merge changes that pass every gate |
| Deliver | Also deploy to approved environments |
| Autopilot | Everything Deliver permits, and Reforge acts on its own |

The first four modes permit actions; people start them. **Autopilot** sets Deliver and has
Reforge start the work: it queues one repair at a time for every open finding, existing
ones included, and requests a merge once every gate passes. It picks the priced model connection with the lowest
cost weighted by its active runs, skipping connections whose own budget is used up or
whose latest call in the past 30 minutes failed, and uses the repository's runner pool, and waits while the organisation budget in
[Usage](usage.md) has no headroom. Blocked findings wait until the blocker is cleared, and the status line under Mode names it.
A failed fix is retried after ten minutes, up to six runs per finding version; the fifth and
sixth runs use the most expensive priced model connection. A finding
it cannot fix is skipped; a new version of the finding is tried again. If a Reforge pull
request is closed without merging, the finding is fixed again from scratch. The status line
shows what it is doing.
A blocked run is resumed, up to three attempts, then cancelled so the finding is queued again.

Only an owner with access to all repositories can turn on Autopilot. Its actions are recorded
under that owner.

## Select a policy

Open **Policies**, choose a repository, then choose **Organisation**, **Team** or
**Repository** scope. Effective summary shows inherited layers, environment access,
named caps and blocking constraints. Version rail lists saved versions; select one to
load it into editor. Long identifiers are shortened in list and remain available on
hover.

## Edit and save

Use **Scope**, **Recipes**, **Changes**, **Models & spend**, **Merge** and **Deploy** tabs.
Reason and **Save immutable version** remain available on every tab. Saving creates a new
version; it does not overwrite history. Draft stays local until explicitly saved.

**Changes** toggles each denied action. Allowlists on **Recipes**, **Models & spend**,
**Merge** and **Deploy** either **Inherit** from the scope above or use an **Allow list**;
an empty allow list denies all. **Pause this scope** stops automatic actions for the scope.

Provide a reason, then select **Save immutable version**. Raw policy JSON is available
under **Advanced policy JSON** for exceptional cases.

## Impact preview

After saving a candidate version, use **Impact preview** to choose **repair**, **publish**
or **merge**, then select **Run impact preview**. Preview reads stored repository heads,
stored findings or changes and effective policy evidence. It does not call providers,
activate policy or mutate repositories.

Coverage is shown as complete or partial. Load more repositories or stored evidence, then
run preview again. Missing, stale or incomplete evidence, missing candidate binding,
unknown effective policy or provider state leaves results blocked or unknown; preview never
fabricates gate changes or enables mutation. A dirty draft disables preview until saved.
Manual **Open simulation** remains available for the selected repository as a separate
workflow.

## Simulate and activate

After saving immutable version, select **Open simulation** in **Candidate simulation** and
provide action, recipe,
model, route, environment, workflow, changed paths, caps, binding and evidence. Run
**Simulate candidate rollout**. Simulation is read-only and returns decision, blockers,
required actions and hashes.

Run simulation, then select **Activate exact simulation**. Activation requires a matching
simulation hash and compare-and-swap version; configuration problems block activation.
Any policy, scope or binding change makes simulation stale. Unknown evidence or provider
state blocks repository mutations.

Inheritance cannot be weakened by lower scope. Zero-valued caps remain enforced and remain
zero when saved. Paused or blocked scopes stay visible; unknown states never imply
permission.
