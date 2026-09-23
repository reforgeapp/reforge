# T28b repository scale slice

Status: local fixture-backed scale slice passed 2026-09-23. T28b remains open.

## Run

`go test -race -count=1 -run '^TestInventoryTenThousandRepositoriesAcrossTenOrganisations$' ./test/integration -v`

Passed on Go 1.27.1/linux-amd64, WSL2 Linux 6.18.33.2, disposable PostgreSQL 18.6. Database container capped at 2 GiB, 2 CPUs and 256 PIDs. Test deadline 8 minutes; outer command timeout 9 minutes. The disposable database was migrated with the current schema and runtime grants, then removed after the run.

Ten organisations each scanned and imported 1,000 repositories through the asynchronous inventory job API and database-backed service. Fixture provider supplied 100 pages per phase (100 scan, 100 import), with one 100-item page per organisation per round. Scan and import together took 25.48 seconds; fixture setup excluded. Every job completed with 1,000 processed rows and 10 pages. Pagination read all 10,000 rows in 50 service calls. Five searches per organisation completed in 50 calls.

Measured warm sequential database-backed service calls:

| Operation | Samples | p50 | p95 |
| --- | ---: | ---: | ---: |
| Repository list page | 50 | 5.29 ms | 6.95 ms |
| Repository search | 50 | 3.74 ms | 4.21 ms |

Ten cross-organisation repository reads were denied. The test made no external provider requests. Raw run summary: [run.json](../../.local/t28b-repository-scale/run.json). Test: [inventory_scale_test.go](../../test/integration/inventory_scale_test.go).

## Limits

This checks database-backed inventory scan/import, pagination, search and tenant scope at 10,000 fixture rows. Provider pages came from the integration fixture reader; this is not live GitHub/GitLab/Gitea certification. Worker steps were driven one at a time in fixed round-robin order, so timing does not establish production scheduler fairness or throughput under contention. No concurrently executing repair runs, cancellation, restart recovery, cross-tenant simultaneous load, hosted isolation, resource peaks or G5 certification were exercised. SQL statements were not counted individually; recorded counts are service-level page/search operations. T28b still needs 100 cgroup-enforced executing runs, 50 browser sessions (already separately recorded), contention-based fairness, cancellation and restart evidence on an adequately delegated runner host.
