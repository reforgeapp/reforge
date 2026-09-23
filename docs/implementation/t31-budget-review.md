# T31 dispatcher accounting evidence

Reviewed 2026-09-23 against [T31 runtime contract](t31-custom-runtime.md).

## Check

With `.local/development.env` sourced:

```sh
go test -count=1 -v -run '^TestCustomProfileReportBudgetAccounting$' ./test/integration
```

Both subcases passed against the fixture-enforced disposable PostgreSQL database `reforge_test`. Output: `.local/t31-budget/postgres-budget.log`.

`TestCustomProfileReportBudgetAccounting` creates and approves a profile, configures a quota route and org limit, enqueues a repair, claims a real runner lease and authorizes through `customcmd.Dispatcher`. While its run remains active, a second authorize is rejected by the profile concurrency cap and creates no second reservation.

It then sends accounting-only report inputs with `state=failed` and an explicit fixture reason; no process result or successful execution is claimed. Known usage settles the durable reservation with exact token, time and request amounts, releases held capacity, and replays without increasing spend. Unknown usage marks the reservation unknown, keeps its full maximum held, and replays without increasing the hold. Revoking the approved profile then blocks another dispatcher authorization.

## Limits

This covers the database-backed dispatcher and accounting boundary only. Report inputs are synthetic; no runtime process, protocol event, provider API, or actual billed usage is involved. The cap case proves profile concurrency denial, not a one-request budget exhaustion. Expired approval is not exercised through dispatcher authorization here. Native publication and end-to-end repair remain covered separately; this test does not certify them.

Confidence: 95% that these checks establish the stated local dispatcher accounting behavior; no claim of real process or provider accounting.
