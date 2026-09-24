# Usage and budgets

The **Usage** route separates settled, reserved and unknown usage. Unknown usage is never
rendered as zero.

## Ledger

Usage records show provider, recipe, state and either the settled amount or the held
maximum. Records can be filtered by team, repository, provider, recipe, connection, state
and time range.

## Charts

All charts follow the active ledger filters. Without a date filter they cover the last 30
UTC days; ranges longer than 366 days are rejected.

| Chart | Shows |
| --- | --- |
| Headline tiles | Settled API cost estimate, recorded tokens, reserved holds and unknown maximum |
| Daily usage | Settled cost, settled tokens or reservation records per UTC day |
| By provider | The selected measure per provider |
| Reservation states | Settled, dispatched, reserved, unknown and cancelled reservations |

Cost is an estimate from the pricing version on each route, not an invoice.

## Budgets

Budgets are hierarchical and can be set for an organisation, team, repository, connection
or campaign. Only an organisation owner can change a budget. A budget edit creates a new
version; periods are immutable once applied.

Open the **Budgets** tab, choose a named scope, and edit the caps. Organisation, team,
repository, connection and campaign scopes use selectors backed by persisted inventory.
The **Usage** tab keeps ledger filters separate from budget editing. USD caps accept six
decimal places and preserve zero explicitly. Each saved non-zero cap shows settled spend
and current holds against the cap.

Contending jobs cannot overspend a configured ceiling. When provider usage is unknown the
reservation is held as unknown until reconciled, which can pause affected automation.

## Subscription routes

A subscription route is shown as eligible, verification required, quota unknown, exhausted
or disconnected. It is never advertised as free, and exhaustion never silently switches to
a paid API route. See [Model routes](models.md).
