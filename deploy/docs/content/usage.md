# Usage and budgets

The **Usage** route separates settled, reserved and unknown usage. Unknown usage is never
rendered as zero.

## Ledger

Usage records show provider, recipe, state and either the settled amount or the held
maximum. Records can be filtered by team, repository, provider, recipe, connection, state
and time range.

## Budgets

Budgets are hierarchical and can be set for an organisation, team, repository, connection
or campaign. Only an organisation owner can change a budget. A budget edit creates a new
version; periods are immutable once applied.

Contending jobs cannot overspend a configured ceiling. When provider usage is unknown the
reservation is held as unknown until reconciled, which can pause affected automation.

## Subscription routes

A subscription route is shown as eligible, verification required, quota unknown, exhausted
or disconnected. It is never advertised as free, and exhaustion never silently switches to
a paid API route. See [Model routes](models.md).
