# T28c clean image review

Built from clean detached worktree at `9a39e8737c8ac90995eef82fda16bd8d48579729` (`9a39e87`, 2026-09-23), platform `linux/amd64`. Worktree had no tracked or untracked changes before build. Docker CLI was sole host build tool for the no-cache pass: `PATH` contained only its directory; Go, Node, Python and Python 3 were unavailable. All compilers/package installers ran inside pinned Docker build stages.

## Final images

| Image | Pinned `FROM` inputs | First build ID | No-cache, Docker-only-PATH ID | APK records |
| --- | --- | --- | --- | ---: |
| Control | `node:26-bookworm@sha256:2aaae6d91f99fee84cfc92da9b52c22a185752d247746052bbc3f961e44478c6`; `golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195`; `alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507` | `sha256:973517b8c4bfc778070040d5552cf7e1a37a7c22dd6b7eb59c1173cbfdfec05a` | `sha256:71e057971429ad36d6b0fb9c393213adab7d70b1c1a6ec371d74bd3b3dff72a5` | 16 |
| Runner | `golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195`; `alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507` | `sha256:9016ddc043d7dc3ae13d59a5c60bd0a64d7c0a529a899719e269f506ccaa3eed` | `sha256:73afb45d544ea6778647efc32e40aa9941781743b58aa305e3be1da1e1ccf6b4` | 28 |
| Docs | `python:3.14-slim@sha256:caaf356f40667c496d405780745b9ac25771c189a51dfcc42430d531ea09f8a2`; `nginx:1.27-alpine@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10` | `sha256:407a90831a1f8328022f33e89327571c7c2708c9aeb64689cd450c7e990b6c52` | `sha256:502861a7c4161710b5bfdd499fba43050387fe66693301b591f7d8d1ded996e4` | 68 |

These are local Docker image IDs, not published OCI digests. All builds succeeded. Control and runner compile Go in `golang` stages; control builds React in `node`; docs installs and strictly builds MkDocs in `python`, then copies only `/site` into nginx.

## Bundle source match

Extracted `/app/web/dist` from clean control image and `/src/web/dist` from a separate build of control Dockerfile's `web` stage at same HEAD. File hashes matched exactly. Repeated for the no-cache control image; hashes still matched.

| File | SHA-256 |
| --- | --- |
| `assets/index-Cp7V3BTd.css` | `c697db88524244cf6e29fba8ffaaa7dd5fb89513a6efd32699de271723c45328` |
| `assets/index-Crtub4So.js` | `a9f32933dce579f0ec96f87748b021deaccf4de7ac2804c03401053bf83e5adb` |
| `index.html` | `3a667da2a332e83201100ae8b17ba0a0ee5852672c90d80af944fb7c52b691a7` |

This closes exact frontend-bundle-to-source-build provenance for this committed revision. Package-level frontend notice reconciliation remains as recorded in [the release attribution map](t28c-release-attribution.md).

## Docs build and HTTP smoke

`deploy/docs/Dockerfile` build stage completed `pip install --require-hashes` and `mkdocs build --strict`. Locked file contains 31 pinned distributions; build-stage `pip list --format=freeze` contained those 31 plus unpinned `pip==26.2.1` supplied by Python base. Final nginx image has no `python`/`python3` executable and has generated search index.

With isolated container `DOCS_VERSION=9a39e87` on `127.0.0.1:18082`, all returned HTTP 200:

| Request | Bytes |
| --- | ---: |
| `/docs/` | 24,661 |
| `/docs/search/search_index.json` | 51,795 |
| `/docs/versions.json` | 34; body `{"versions":["latest","9a39e87"]}` |
| `/docs/9a39e87/` | 24,661 |

`/docs/version.json` and `/versions.json` were exploratory wrong paths and returned 404; configured application route and version index above pass.

## Reproducibility and limits

Two builds from same clean commit/platform and pinned `FROM` images produced different complete control, runner and docs image IDs. The frontend files match byte-for-byte and package-record counts match. Complete image reproducibility therefore remains unproven; this run did not isolate the layer differences or freeze Alpine repository snapshots. Buildx was absent, so Docker used deprecated legacy builder; multi-platform manifest production was not exercised. No arm64 build ran.

Root review identified `.dockerignore` omits root `.env` and nested secret env files while Go build stage runs `COPY . .`. No secret file was present in the clean worktree, but the builder layer/cache exclusion is not established. Remediate separately and rebuild/review before distributing build cache.

OS package source archives and required notice texts remain incomplete. Docs asset/npm notice applicability and exact full shipped texts still need reconciliation; 28 npm gaps are development-only but builder-distribution scope remains. `mergedeep` metadata still reports `UNKNOWN` despite retained MIT source text. No license interpretation or legal/owner signoff performed. Arm64 and external registry publication provenance remain open.

## Commands and evidence

Commands were run from the clean worktree:

```sh
git worktree add --detach .local/t28c-clean-images/source 9a39e8737c8ac90995eef82fda16bd8d48579729
docker build --pull --platform linux/amd64 -f deploy/control/Dockerfile -t reforge-t28c-clean-control:9a39e87 .
docker build --pull --platform linux/amd64 -f deploy/runner/Dockerfile -t reforge-t28c-clean-runner:9a39e87 .
docker build --pull --platform linux/amd64 -f deploy/docs/Dockerfile -t reforge-t28c-clean-docs:9a39e87 deploy/docs
PATH=/home/mnorris/repos/reforge/.local/t28c-clean-images/docker-only-bin docker build --pull --no-cache --platform linux/amd64 -f deploy/control/Dockerfile -t reforge-t28c-clean-control:9a39e87-isolated-path .
PATH=/home/mnorris/repos/reforge/.local/t28c-clean-images/docker-only-bin docker build --pull --no-cache --platform linux/amd64 -f deploy/runner/Dockerfile -t reforge-t28c-clean-runner:9a39e87-isolated-path .
PATH=/home/mnorris/repos/reforge/.local/t28c-clean-images/docker-only-bin docker build --pull --no-cache --platform linux/amd64 -f deploy/docs/Dockerfile -t reforge-t28c-clean-docs:9a39e87-isolated-path deploy/docs
```

Ignored logs, HTTP captures, extracted bundles, Python graph, APK counts, and image inspection output are under `.local/t28c-clean-images/artifacts/`. This review introduces no code or image distribution, and makes no release-notice or legal-compliance claim.
