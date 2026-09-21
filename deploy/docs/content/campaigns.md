# Campaigns

A campaign applies one recipe to an explicit, pinned set of repositories. It is not a
saved filter that grows.

## Planning

1. Choose the kind: repair, native pipeline or GitOps.
2. Select repositories. The selection is snapshotted; later filter changes do not add
   members.
3. Set canary size, batch size, concurrency, the success criterion, the observation
   window and failure thresholds.
4. Preview exclusions, blockers, canary groups and the campaign budget.
5. Create the campaign, then **Start canary**.

Repair campaigns require a configured campaign budget before they can start.

## Lifecycle

- Canaries run first and must cover every recipe, forge-version and validation group.
- Any canary failure stops expansion and requires a new campaign with renewed canaries.
- Expansion proceeds one bounded stage at a time. Changing pins, losing authority or
  exceeding a failure threshold pauses the campaign.
- Pause and cancel request qualified native cancellation for pending work. Already
  completed merges or deployments are not rolled back; recovery uses individual recorded
  operations.
- Resume requires a reason and continues the current stage.

Members link to their ordinary run, deployment or promotion records.

## Fairness

The controller rotates organisations and campaigns and releases database locks before
provider I/O, so one large campaign does not starve another organisation. A 1,000-repository
snapshot is exercised in tests.
