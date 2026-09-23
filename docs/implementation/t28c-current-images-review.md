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

## Final frozen-source verification

Date: 2026-09-24. Frozen source: clean detached worktree at `675b0fdc0bb315c44506dc417cfa1f686d116a4a` (`.local/opus-resume/current-images/source-675b0fd`). `source.txt` records exact HEAD and clean tracked status. Rebuilt for `linux/amd64` with `--pull=false`, legacy Docker builder, and the local Docker-only PATH. Full commands and logs are under `.local/opus-resume/current-images/artifacts/final-675b0fd/`.

| Image | Image ID | User |
| --- | --- | --- |
| `reforge-t28c-final-control:675b0fd` | `sha256:59819fbd3a751b824117728157934a1b75af1228901848d1c9794c5499f17da0` | `reforge` (10001) |
| `reforge-t28c-final-runner:675b0fd` | `sha256:53438610f14b72d13bb4ba525c1dacf0a46220a6506340d0f221ee07b3627af9` | `runner` (10002) |
| `reforge-t28c-final-docs:675b0fd` | `sha256:7ad3cbc38c7fbe740c60f3969693368781d8ad6052f3f0a8f5c2e09046efc69c` | `101:101` |
| `reforge-t28c-final-web:675b0fd` | `sha256:f926f8940ce1bf50443bc18043b7b35700c50294fd73af31128061db6230857e` | build stage |

Control and web-stage frontend files match byte-for-byte: JS `83b9348b17c7095e3d97b49144145ed6c1db4cfbb76ef990e426154072c91dc1`, CSS `c15f147c4623bfc3a8a7078b1b2b3c64d456cf8934f29cddf11fc332d536ca6e`, HTML `44cb1d530448c1bfdb06d4a258ff3525b79c79f710dbdac879aa9c08788289ec`. Notices in all three runtime images match source: 1,475,679 bytes, SHA-256 `49413aee16d4d7983871e92d592a5047ae2fedcf3b593fe10898bcb4354b3af7`. Control binaries are executable; migration `036_org_oidc_invitations.sql` is present. Strict MkDocs build passed; docs index and search index hashes remain `3ec1936a3a76bcfa4fd6fa0872823fe7a9da615f618776f2f58c10d000ea143b` and `749a1bda6412dfbc7634a73341a9307213de0e792edfb59cf46aa8d8e0e8c996`. The first docs build invocation used worktree root as context and failed because `requirements.lock` is in `deploy/docs`; corrected build used `deploy/docs` context and passed. Both logs are retained.

The original tracked `scripts/container-install-check.sh` ran directly from the clean worktree with its default `.local/t28a-install` output path (ordinary directory; no path edits). Exit status 0. It passed clean install/migration and least-privilege grants, additive migration, failed-migration rollback and retry/idempotency, encrypted credential handling, local fixture OIDC discovery/login/session, process restart, database dump/restore, restored-schema migration, decryption using the backed-up key, and restored-database OIDC login. Run log: `.local/opus-resume/current-images/artifacts/final-675b0fd/container-install-check.log`; complete run evidence: `.local/opus-resume/current-images/source-675b0fd/.local/t28a-install/t28a_20260923150204_22669/`. Harness-built control image ID: `sha256:8ae14d714e3949aba417147873448cebf7fddc7fe9f8054261a75c89abfff0d0`.

Final docs image smoke returned HTTP 200 for `/docs/`, `/docs/675b0fd/`, `/docs/versions.json`, `/docs/search/search_index.json`, `/docs/admin/`, and `/docs/third-party-notices.txt`; served notice bytes match image/source. The demo on `http://127.0.0.1:8082/docs/` now uses final image ID `sha256:7ad3cbc38c7fbe740c60f3969693368781d8ad6052f3f0a8f5c2e09046efc69c`, container `reforge-t28c-current-docs-serve`, UID 101, restart unless-stopped, read-only root, 64 MiB noexec/nosuid/nodev `/tmp`, loopback-only binding. Stop with `docker stop reforge-t28c-current-docs-serve`. Container identity and smoke evidence are retained in the final artifacts directory.

Final verification used only local amd64 images and disposable local fixtures. No registry publication, arm64 build, external G5, hosted IdP, customer TLS, or KMS topology was tested. No source implementation changes or commits were made for this verification.

Confidence: 97% for frozen-source local builds, byte checks, strict docs build, non-root runtime users, docs routes, and direct install/recovery harness. No broader release or provenance certification claim.
