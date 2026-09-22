# Policies

Policy controls automatic actions. Policies are immutable versions scoped to the
organisation, team or repository.

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
