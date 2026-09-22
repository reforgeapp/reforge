# Policies

Policy controls what Reforge may do automatically. It is versioned and scoped to the
organisation, a team or a repository.

## Inheritance

A repository inherits the organisation baseline and any team overlays. The editor shows
inherited values and the effective result, including which higher-level rule prevents
loosening. A candidate repository policy cannot activate itself.

## Editor tabs

The editor uses **Scope**, **Recipes**, **Changes**, **Models & spend**, **Merge** and
**Deploy** tabs. Structured controls are the primary workflow. Raw policy JSON is available
only under advanced import/export.

## Simulation and rollout

Choose **Open simulation** to reveal rollout inputs, then run **Simulate candidate rollout**.
For the selected repository, provide the action, recipe, model, paths, binding and evidence
inputs. The simulation returns the candidate decision, policy hash and blockers without
calling tools or mutating providers. Denials carry the rule that produced them. Named preset
workflows are not currently exposed; configure the structured policy fields directly.

Unknown never allows a mutation. A pause prevents new application actions; already
admitted native actions are observed or cancelled where the provider supports it, without
promising reversal.
