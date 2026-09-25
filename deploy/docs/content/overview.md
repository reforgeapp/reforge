# Overview

The **Overview** route summarises the organisation within the viewer's repository scope.
Every count links to its filtered list.

## Get started

Shown until every step is done; each step links to where it is fixed.

| Step | Done when |
| --- | --- |
| Connect your code host | A forge connection is healthy |
| Import a repository | At least one repository is imported |
| Scan a repository | A discovery scan has completed |
| Add an AI model | A model connection is healthy |
| Set model pricing | A healthy model has a priced billing route |
| Start a runner | A runner reported in the last 5 minutes |
| Give a runner your repositories | An active runner pool includes a repository |
| Allow Reforge to propose fixes | The effective policy permits repair |
| Enable fixes on this server | The operator configured `REFORGE_REPAIR_IMAGES` |

## Counts

| Tile | Counts |
| --- | --- |
| Needs decision | Open findings |
| Running | Active tasks, merge operations and deployments |
| Ready for review | Published repair runs |
| Blocked | Blocked tasks, merge operations and deployments |
| Verified deployments | Deployments currently healthy |

Stale repositories (never synced, or not synced for 24 hours) are shown separately.

## Activity

Stacked daily columns for the last 14 UTC days:

| Series | Counted on |
| --- | --- |
| Findings | Day the finding was first seen |
| Runs | Day the repair run started |
| Merged | Day the merge operation reached `merged` |
| Healthy deployments | Day the deployment first reported healthy |

Hover or focus the chart and use the arrow keys to read a single day.

## Open findings

Open findings grouped by severity, highest first.
