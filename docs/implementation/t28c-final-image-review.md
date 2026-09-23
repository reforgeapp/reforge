# T28c final local image review

Built from clean detached commit `cce6879a0d04e39ce13020fedd37f58a333d0f3f` on 2026-09-23 for `linux/amd64`. `git status --porcelain` was empty before build. Each Docker build ran with host `PATH` limited to Docker CLI; Docker used its legacy builder and cached unchanged layers. Dockerfiles pin every `FROM` by digest. Images remain local and unpublished.

## Images

| Image | Local image ID | Config user | Result |
| --- | --- | --- | --- |
| `reforge-t28c-final-control:cce6879` | `sha256:3ea417c6f6a5686d8941757e045334fb1da9b2fddf62f415ba88ba9fb6d04b84` | `reforge` (uid 10001) | Built; server, migrator and evidence binaries executable |
| `reforge-t28c-final-runner:cce6879` | `sha256:b743573e7f3788d0bc211f77128ab1c7a91e38a67b7e32e57b6cad1614564bf4` | `runner` (uid 10002) | Built; runner binary executable |
| `reforge-t28c-final-docs:cce6879` | `sha256:2885ac440c48972efd122352993382b7349bb5eeb72e505029cf6a58c5fecf78` | unset (defaults to root) | Built; strict MkDocs build passed; generated search index present; no Python runtime |

Build logs, image IDs, extracted binaries and static site files are under ignored `.local/t28c-final-images/artifacts/`. Clean detached source is retained under ignored `.local/t28c-final-images/source/`.

## Checks

- Docker `COPY` probes against disposable marker files failed with `file not found in build context or excluded by .dockerignore` for root `.env`, `web/.env`, and `deploy/compose/runner/credentials/review-marker`. Markers were removed before product image builds; source worktree was clean.
- Control frontend extracted from `/app/web/dist` byte-matches separate `web` stage output. Hashes: CSS `417cbefa297576f72efcf32498c25ca68ea3b4c4f5a7cde0f7ea6e87e9477c72`; JS `68e9e0405ab4c8b8834922adf029d207a0db45bc3b3a665de764514683f9595d`; HTML `b6db1c5c98a70903bb32676871b7529b2fb05296a829b3378da22898413b546a`. Full sorted manifests match in `control-frontend.sha256` and `web-stage-frontend.sha256`.
- Notice file in control and runner at `/usr/share/doc/reforge/third-party-notices.txt`, and docs image at `/opt/docs-site/third-party-notices.txt`, each byte-matches canonical source: SHA-256 `49413aee16d4d7983871e92d592a5047ae2fedcf3b593fe10898bcb4354b3af7`.
- Docs HTTP smoke on loopback passed: `/docs/`, `/docs/cce6879/`, `/docs/versions.json`, `/docs/search/search_index.json`, `/docs/third-party-notices.txt` returned HTTP 200. Served notice bytes match image and source. Response sizes and captures are in `docs-http-smoke.tsv` and `http-*.out`.
- Extracted binary hashes: `reforge` `1c6d16fc1d927f8021576ed048e4247446305406ccbd96739c7c3f6ec9b5b963`; `reforge-migrate` `367c06fa3b22e13c91f87ac5c7029218fe4c1cdfe14cce18d286573ff180d4c6`; `reforge-evidence` `7d85fb85afb438f70416d90f441256faf11ed3835f573d6f8b36aad01c793538`; `reforge-runner` `7a0ed8d0f294e1ce5699e8ef1899d90059919e4a016c388cb4c926add6f8a5f1`. Docs `index.html` hash `3ec1936a3a76bcfa4fd6fa0872823fe7a9da615f618776f2f58c10d000ea143b`; search index hash `5991c8a50905bd511dddae9bfa5c770a7f27433b707d95d394ebf26e0f7ab925`.
- Final build used pinned cached inputs (`--pull=false`); no arm64 image build ran. Disk free before build was 9.0 GB, after 7.2 GB. Temporary docs smoke container was stopped and removed. No shared images or Docker cache were pruned.

## Limits

These are local image IDs, not published OCI manifest digests. Full image reproducibility remains unproven: [the prior same-commit review](t28c-integrated-image-review.md) records differing IDs across two builds of `9a39e87`. OS package source archives and full license/notice attribution remain incomplete; no legal clearance was performed. Arm64, registry publication provenance, and external G5 certification remain open. Docs image runs with unset `Config.User` (root default); non-root verification passed for control and runner.

## Commands

Run from clean detached source at `cce6879a0d04e39ce13020fedd37f58a333d0f3f`:

```sh
PATH=/home/mnorris/repos/reforge/.local/t28c-final-images/docker-only-bin docker build --pull=false --platform linux/amd64 -f deploy/control/Dockerfile -t reforge-t28c-final-control:cce6879 .
PATH=/home/mnorris/repos/reforge/.local/t28c-final-images/docker-only-bin docker build --pull=false --platform linux/amd64 -f deploy/runner/Dockerfile -t reforge-t28c-final-runner:cce6879 .
PATH=/home/mnorris/repos/reforge/.local/t28c-final-images/docker-only-bin docker build --pull=false --platform linux/amd64 -f deploy/docs/Dockerfile -t reforge-t28c-final-docs:cce6879 deploy/docs
PATH=/home/mnorris/repos/reforge/.local/t28c-final-images/docker-only-bin docker build --pull=false --platform linux/amd64 --target web -f deploy/control/Dockerfile -t reforge-t28c-final-web:cce6879 .
```

Confidence: 97% for local image build, notice delivery, frontend match, non-root control/runner, and docs route checks. No claim of complete legal attribution or G5 certification.
