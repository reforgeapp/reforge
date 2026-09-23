# T28c current image review

Date: 2026-09-24. Frozen product source: clean detached `18c8045d2b0751aaf8488132c93cc7525bb5032b`, built for `linux/amd64`. Main worktree was dirty in unrelated files; none entered this build context. Docker used `--pull=false`, Docker-only host `PATH`, pinned bases and legacy builder. Missing Buildx plugin emitted a metadata warning; all builds completed.

## Images

| Image | Local image ID | Config user | Result |
| --- | --- | --- | --- |
| `reforge-t28c-current-control:18c8045` | `sha256:80f66adaf351338158c17d706cfdda3a870c6a71f5aba6e0b3c98f5f7e31bf25` | `reforge` (uid 10001) | Built; server, migrator, evidence executables verified |
| `reforge-t28c-current-runner:18c8045` | `sha256:53438610f14b72d13bb4ba525c1dacf0a46220a6506340d0f221ee07b3627af9` | `runner` (uid 10002) | Built; runner executable verified |
| `reforge-t28c-current-docs:18c8045` | `sha256:7ad3cbc38c7fbe740c60f3969693368781d8ad6052f3f0a8f5c2e09046efc69c` | `101:101` | Strict MkDocs build; nginx site and search index present |
| `reforge-t28c-current-web:18c8045` | `sha256:62c83b751cef7b767c5d72b296f058719b2bf493f0d564d46255791c87a2dd81` | unset | Separate frontend comparison stage |

These are local Docker image IDs, not registry manifest digests. Build commands, complete logs, extracted image contents, IDs and hashes are in ignored `.local/opus-resume/current-images/artifacts/`.

Control image frontend byte-matches the separately built `web` stage: all three files match. CSS `c15f147c4623bfc3a8a7078b1b2b3c64d456cf8934f29cddf11fc332d536ca6e`; JS `4fb3b1c79080c96aacee60e3491654b3e529d83595824c3c36684379537f79e3`; HTML `52280302874323d1cb14f1e6e6195668a75c328fcc0dda5b144db63bb527db09`. Notice files in control, runner, and docs each match canonical source byte-for-byte: 1,475,679 bytes, SHA-256 `49413aee16d4d7983871e92d592a5047ae2fedcf3b593fe10898bcb4354b3af7`.

Strict MkDocs build passed. Docs index SHA-256 is `3ec1936a3a76bcfa4fd6fa0872823fe7a9da615f618776f2f58c10d000ea143b`; search index is `749a1bda6412dfbc7634a73341a9307213de0e792edfb59cf46aa8d8e0e8c996`. Container and live smoke returned HTTP 200 for `/docs/`, `/docs/18c8045/`, `/docs/versions.json`, `/docs/search/search_index.json`, `/docs/admin/`, and `/docs/third-party-notices.txt`; served notice bytes match the image and source.

The docs image is serving the production demo on `http://127.0.0.1:8082/docs/`. Container ID `3bc8f95efb081744eda9f89ac147e1873b20cb02818b5ac94faeee2887f86b44`, PID 100075, restart policy `unless-stopped`; root filesystem read-only, `/tmp` tmpfs 64 MiB with `noexec,nosuid,nodev`, process UID 101, loopback bind only. Stop command: `docker stop reforge-t28c-current-docs-serve`.

## Install and migration acceptance

The container-only harness passed from this source with the readiness correction described below. Fresh PostgreSQL installed migration set including `036_org_oidc_invitations.sql`, least-privilege roles/grants, and production-mode server. It passed an additive migration, rollback of a deliberately failed migration, retry and idempotency; encrypted credential storage; local signed OIDC discovery/login/session; process restart; `pg_dump` restore; restored-schema migration; decryption with the retained key; and OIDC login against the restored database. No external IdP was used. Final pass log and backup are under the install run directory in the artifacts tree.

The first run failed at role setup because the harness accepted the PostgreSQL image's temporary init server through its default Unix socket probe, then `psql` reported `the database system is shutting down`. Inspection of the exact pinned PostgreSQL image entrypoint confirmed `docker_temp_server_start` sets `listen_addresses=''`; this init server cannot accept TCP. I changed the tracked Compose healthcheck and harness readiness probe to `127.0.0.1`, and made the harness wait for Compose health before role setup.

The final tracked logic passed the full install/restore flow via an artifact copy with only source-root and evidence-output paths redirected. A direct invocation with `.local/t28a-install` symlinked to the owned artifact directory failed before container startup because the script’s `mkdir -p` encountered that existing symlink and returned `mkdir: cannot create directory .../.local/t28a-install: File exists`; that attempt and the earlier PostgreSQL failure are retained. The passing log is `install-final-readiness-copy.log`; the copy and exact tracked diff are retained for review.

The readiness-only tracked diff is two files, 3 insertions and 3 deletions. SHA-256 of `readiness-fix.diff`: `fefffa724c212fcb227df1e0a735c2622542341b03dcec59576ff485d2443cbb`. This diff is separate from the product image source identity; it does not change the built image contents. No commit was created.

Completed install-run Go module/build caches were removed after each run exited; cache paths, allocated bytes and totals are recorded in `cache-reclamation.txt`. Logs, source, dump and fixture evidence remain. Disk free after cleanup was 4.7 GiB.

## Limits

Images are local amd64 artifacts; no arm64 build, registry publication, external G5 acceptance or hosted IdP/TLS/KMS topology was tested. Existing Alpine review still records incomplete package-source archives and full license/notice attribution; no legal clearance was performed. The docs image runs non-root. The browser worker may use the local docs service; the positive-merge worker was notified that disk headroom is available and no model files were downloaded by this run.

Confidence: 96% for local image builds, byte checks, non-root execution, docs routes and disposable install/recovery acceptance. No broader provenance or release certification claim.
