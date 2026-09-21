# Policies

Policy controls what Reforge may do automatically. It is versioned and scoped to the
organisation, a team or a repository.

## Inheritance

A repository inherits the organisation baseline and any team overlays. The editor shows
inherited values and the effective result, including which higher-level rule prevents
loosening. A candidate repository policy cannot activate itself.

## Presets

- **Observe** — discovery and evidence only.
- **Propose fixes** — publish changes, never merge.
- **Merge eligible fixes** — merge when native gates and policy allow.
- **Deliver to approved environments** — request allowlisted delivery.

Presets are editable configurations, not hidden feature tiers. Moving to a more permissive
preset requires the policy administrator role and an audit reason.

## Simulation and rollout

Simulation evaluates stored findings and changes without calling tools or mutating
providers. The apply preview lists affected repositories and proposed gate changes before
activation. Denials carry the rule that produced them.

Unknown never allows a mutation. A pause prevents new application actions; already
admitted native actions are observed or cancelled where the provider supports it, without
promising reversal.
