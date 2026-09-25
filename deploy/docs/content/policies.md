# Policies

Policy controls automatic actions. Policies are immutable versions scoped to the
organisation, team or repository.

## Mode

**Mode** sets the organisation policy in one step. It saves, simulates and activates a new
organisation version.

| Mode | Permits |
| --- | --- |
| Observe | Read only |
| Propose fixes | Repair and publish changes |
| Merge eligible fixes | Also merge changes that pass every gate |
| Deliver to approved environments | Also deploy |

Mode permits actions. **Fix findings automatically** starts them: Reforge queues one repair
at a time for every open finding, existing ones included, and with **Merge eligible fixes**
requests a merge once every gate passes. It uses the first priced model and the
repository's runner pool, and waits while the organisation budget in
[Usage](usage.md) has no headroom. Blocked findings wait until the blocker is cleared, and the line under the switch names it.
A failed fix is retried after ten minutes, up to three runs per finding version. A finding
it cannot fix is skipped; a new version of the finding is tried again. The line under the switch shows what it is doing.

Only an owner with access to all repositories can turn it on. Its actions are recorded
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

Presets update draft only. They add draft denials while preserving existing read denials,
allowlists and higher-scope constraints:

- **Observe** allows read and blocks repair, publish, merge, deploy and recover.
- **Propose fixes** allows read, repair and publish; blocks merge, deploy and recover.
- **Merge eligible fixes** allows read, repair, publish and merge; blocks deploy and recover.
- **Deliver to approved environments** allows read, repair, publish, merge and deploy;
  blocks recover. Existing environment and workflow allowlists remain unchanged. Empty
  explicit lists deny all.

Provide a reason, choose a preset, select **Apply preset**, review the draft gate, then
select **Save immutable version**. Raw policy JSON is available under **Advanced policy
JSON import/export** for exceptional cases.

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
