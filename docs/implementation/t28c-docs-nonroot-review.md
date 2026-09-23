# T28c docs image non-root review

Docs runtime now uses UID/GID `101:101`, serves on container port `8080`, stores generated versioned files and nginx temporary state under `/tmp`, and has a read-only root filesystem with a 64 MiB bounded `/tmp` in Compose. Compose keeps host binding `127.0.0.1:8082`. No docs-service volume, Docker socket or secret mount was added.

## Verification

Built local linux/amd64 image `reforge-t28c-docs-nonroot:2026-09-23`, ID `sha256:2c1294d7c67170366484b5cfb934dbbcd0c7e8517cf3d332af4ee20f43a6ce2f`:

```sh
docker build --pull=false --platform linux/amd64 -f deploy/docs/Dockerfile -t reforge-t28c-docs-nonroot:2026-09-23 deploy/docs
```

The cached strict MkDocs build completed. Docker used its legacy builder because local Buildx executable is missing.

Ran isolated container on `127.0.0.1:18082` with `--read-only --tmpfs /tmp:rw,noexec,nosuid,nodev,size=64m,mode=1777` and `DOCS_VERSION=t28c-nonroot`. Image config is `User=101:101`; both nginx processes ran as UID 101. Container had no bind mounts or named volumes; `/tmp` was a bounded tmpfs. All routes returned 200: `/docs/`, `/docs/search/search_index.json`, `/docs/versions.json`, `/docs/t28c-nonroot/`, `/docs/third-party-notices.txt`. Version index was `{"versions":["latest","t28c-nonroot"]}`. Bundled notice SHA256 and served response SHA256 both matched `49413aee16d4d7983871e92d592a5047ae2fedcf3b593fe10898bcb4354b3af7`.

`docker compose --env-file deploy/compose/.env.example -f deploy/compose/compose.yaml config --quiet` passed. Existing shared `http://127.0.0.1:8082/docs/` still returned 200. The isolated smoke container was removed.

## Limits

Only linux/amd64 built locally. Buildx was unavailable, so this did not verify BuildKit, arm64 or published image behavior. OS package source notices, complete reproducibility and legal review remain open in the T28c evidence records.
