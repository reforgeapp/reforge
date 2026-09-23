# T28c package and notice review

Reviewed on 2026-09-23 against the local T28c image builds and dependency inventory. This is evidence collection, not legal advice or a licence compatibility decision.

## OCI images inspected

The local images were built from the pinned references in the control, runner, docs, and Compose definitions. `docker image inspect` reported `linux/amd64` and these local image IDs:

| Image | Local image ID | Runtime package evidence |
| --- | --- | --- |
| `reforge-control:t28c-2026-09-23` | `sha256:fdb90334236c0783b9a211230a327294e8e49598f4f40a7b600d2f625ba46f18` | 16 Alpine records; Alpine 3.21.8 |
| `reforge-runner:t28c-2026-09-23` | `sha256:aba618640269d1b76983ccdb4d2958db967c1a30d56849ed32f7bb8d875aeed2` | 28 Alpine records; Alpine 3.21.8 |
| `reforge-docs:t28c-2026-09-23` | `sha256:15f705d4ec720ef23c8b085ff29dc0d1bc8ed223d0008ed4647cc2227199774a` | 68 Alpine records; Alpine 3.21.3; nginx 1.27.5 |
| `postgres:18.6-alpine` | `sha256:c293117fcecda7344b5480222e813b9f673d7abd69b1dd95eff239b768b04f59` | 53 Alpine records; Alpine 3.24.2; one synthetic `.postgresql-rundeps` record has no license tag |

The T28c inventory records the exact six pinned OCI references and local registry digests, including build inputs Node, Go, and Python. The final images inspected here correspond to its recorded build IDs. This review checked the final image IDs and their package databases; it did not independently re-query registries or verify signatures for every pinned OCI index.

## Alpine package evidence

Each final Alpine image retains `/lib/apk/db/installed`. The records provide package name, version, architecture, checksum and Alpine `L:` license tag. Collected exact package/version/license-tag rows are in ignored local artifacts:

- `.local/t28c-notices/reforge-control_t28c-2026-09-23.apk.tsv`
- `.local/t28c-notices/reforge-runner_t28c-2026-09-23.apk.tsv`
- `.local/t28c-notices/reforge-docs_t28c-2026-09-23.apk.tsv`
- `.local/t28c-notices/postgres_18.6-alpine.apk.tsv`

The control and runner install `ca-certificates`; runner also installs `git`. Docs final image inherits nginx and its modules. The Alpine package database supplies identifiers and exact versions, not full licence or attribution text. A filesystem search found nginx copyright files under `/usr/share/licenses` in the docs image; control, runner, and Postgres retained no files under `/usr/share/licenses` or `/usr/share/doc`. Therefore current `third-party-notices.txt` does not establish notice coverage for the exact final OS package sets.

Before publishing these images, retain package-source evidence matching each installed APK checksum: package metadata and licence files from the repository snapshot used for the build, plus any package notices required for redistribution. Include all inherited nginx/Postgres packages, not only packages named in this repository’s `apk add` lines. Resolve Alpine tags such as `custom`, `curl`, and compound identifiers from the exact package source metadata; do not map them to SPDX names by guesswork. Add resulting text or a reproducible image-SBOM/notice artifact to release evidence.

## Docs Python build graph

Rebuilt the docs Dockerfile `build` target locally using the existing Docker cache, then inspected installed distribution metadata in that stage. The stage has 30 Python distributions, including `pip` from the base image and `mkdocs-material==9.7.0`; only MkDocs Material is version-pinned in the Dockerfile. The other versions reflect that cached build and are not reproducible inputs because there is no Python lock file. Metadata lists root license-file paths for 29 distributions; `mergedeep 1.3.4` reports `UNKNOWN` and no license file. Exact installed name, version, license metadata, and license-file path values are in `.local/t28c-notices/python-docs-distributions.json`.

The Python environment is confined to the docs build stage; final docs image copies `/site` into nginx and contains no Python environment. However, the generated static site is shipped in the final docs image, and this review did not trace every generated asset back to source package or verify its attribution in the final image. Pin and record the complete Python graph, preserve its exact license/notice texts, then inspect generated site assets and final docs image before release. This is a release evidence gap for the docs image, not evidence that Python runtimes are shipped.

## npm development-only gaps

The generated manifest identifies 28 packages with lockfile license metadata but no top-level license or notice text. All 28 are marked development-only in `web/package-lock.json`; the existing inventory scopes them to frontend build/test, not the production bundle. The Dockerfile runs `npm ci` in a builder stage and copies only `web/dist` into the control runtime image. Thus these missing package-root texts do not, on current evidence, block distribution of installed npm packages in the final control image. They remain build-environment evidence gaps if intermediate builder images/cache are distributed, and should be resolved before such distribution. This does not determine whether any generated asset embeds code from a package; production-bundle provenance remains part of final image review.

## Disposition

Local evidence confirms final image identities, package versions and Alpine metadata tags. It does not provide a complete source-text notice set for OS packages or a locked, fully attributed docs site. T28c notice acceptance remains open until exact final-image SBOM/package-source text, the docs Python lock and notices, and generated-site asset attribution are attached to release evidence. No blanket licence-family, compatibility, telemetry, or legal-clearance conclusion is made.
