# T28b browser load review

Run: 2026-09-23 02:40:51 UTC against `http://127.0.0.1:8080` (`reforge` `0.1.0-dev`). Result: local target met. 50 independent Chromium contexts logged in through explicit development fixture auth, navigated to real Repositories pages, and rendered 25 persisted rows each. Each session had a distinct session cookie; all represented the same seeded owner and organisation. No routes or APIs were intercepted; no external provider was called.

Five measurement rounds ran while all 50 pages remained open. Each round issued 100 parallel authenticated requests against real portfolio list and repository detail endpoints. List: 250 samples, p50 92.42 ms, p95 109.90 ms, max 110.75 ms. Detail: 250 samples, p50 93.81 ms, p95 110.27 ms, max 111.27 ms. Both below product target of 500 ms p95. Zero request/HTTP errors and zero harness retries. Total run time 7.73 seconds.

The endpoints return `Cache-Control: no-store`; measurements are warm repeated persisted reads, not HTTP cache hits. This run establishes the 50-session browser/API slice only. It does not cover 100 executing runs, fairness, cancellation, restart, tenant isolation, hosted isolation or G5. One local fixture tenant and a 25-row portfolio limit representativeness.

Host: WSL2 Linux 6.18.33.2, x64, Node v26.7.0, Chromium 153.0.8010.12, 12 logical CPUs, 16,352,161,792 bytes reported total RAM. Process exposed no cgroup CPU/memory/pid limits (`cpu.max`, `memory.max`, `pids.max` unavailable). Browser workload bounded to 50 contexts, five request rounds, 180-second deadline; all contexts and browser closed in `finally`.

Served identity: HTML SHA-256 `ab28722673ad85d35d1ebb34e3a55ff6834b967a2f7b5ec37f14fa0affaedb09`; JS `/assets/index-CJN-NXVT.js` SHA-256 `a0cfa8b5498243050b77aef60a4f2882988a6da7c99ef1ed0b61789c578367b9`; CSS `/assets/index-Bd1oeqz3.css` SHA-256 `57d8f580a6a1e1ca93749026d0cb102b63d0cd9e972142c5c87911c0dfc0ae37`.

Reproduce from repo root with `node web/scripts/browser-load.mjs` while the local app and PostgreSQL are running. Full request samples, exact metadata and captured failure evidence are under `.local/t28b-2026-09-23/`; successful run report: `.local/t28b-2026-09-23/2026-09-23T02-40-51-491Z/report.json`.
