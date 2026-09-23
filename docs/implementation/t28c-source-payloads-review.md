# T28c source payload provenance review

**Scope:** declared source inputs for the 21 exact aports `(commit, origin)` references in [t28c-current-alpine-inputs-review.md](t28c-current-alpine-inputs-review.md). No notices, Dockerfiles, buildfiles or product files changed. Docs-image gaps remain separate.

## Method

Retained APKBUILDs under `.local/opus-resume/alpine-current/apbuilds/` were parsed as text; none was executed. `source=` entries were expanded only from literal `pkgver`/`_nbver`/`_tagver`-style assignments and matched by filename to `sha512sums=`. `alpine-keys` builds its source list in a loop, so its 18 `sha512sums` filenames were used as the list.

Upstream entries used the declared URL, following HTTP redirects. Aports-local files used the official GitHub mirror path `raw.githubusercontent.com/alpinelinux/aports/<commit>/main/<origin>/<file>`, pinned to the exact commit. No latest version, alternate mirror or repository clone was used. Downloads were streamed with a 250 MiB total cap, a 64 MiB per-file cap and a free-disk floor of 3 GiB + 128 MiB checked before each file.

## Results

| Measure | Value |
| --- | --- |
| References inventoried | 21 |
| Declared files with `sha512sums` | 159 (20 upstream, 139 aports-local) |
| HTTP 200 and declared sha512 match | 159/159 |
| Failures | 0 |
| References with no declared sources | 1 (`alpine-base`) |
| Transferred bytes | 54,980,611 (upstream 54,619,617; local 360,994) |
| Retained storage | 56,001,118 bytes |

| Origin | Commit | Upstream match | Aports-local match |
| --- | --- | --- | --- |
| pcre2 | `0826e91f0c74` | 1/1 | 0/0 |
| libidn2 | `1cdcdd0eccbe` | 1/1 | 0/0 |
| musl | `21cafa1183cf` | 1/1 | 15/15 |
| busybox | `284d827a2793` | 1/1 | 70/70 |
| zlib | `2b38f55109ad` | 1/1 | 0/0 |
| curl | `2c7959533911` | 1/1 | 3/3 |
| libpsl | `398a5aee3025` | 1/1 | 0/0 |
| pax-utils | `398a5aee3025` | 1/1 | 0/0 |
| ca-certificates | `3f566dda324c` | 1/1 | 0/0 |
| c-ares | `40d4af1c03fd` | 1/1 | 0/0 |
| apk-tools | `41847d6ccff0` | 1/1 | 3/3 |
| zstd | `5c2ddf18f193` | 1/1 | 0/0 |
| alpine-baselayout | `5f0cd7890349` | 2/2 | 11/11 |
| alpine-keys | `6d473fb38eff` | 0/0 | 18/18 |
| openssl | `9b59567ddd9c` | 1/1 | 15/15 |
| brotli | `bdeb5ac39445` | 1/1 | 1/1 |
| libunistring | `c4c67852adc5` | 1/1 | 0/0 |
| alpine-base | `c9e7411a5b43` | 0/0 | 0/0 |
| git | `ca82c367c9e5` | 1/1 | 3/3 |
| expat | `d46a3884017c` | 1/1 | 0/0 |
| nghttp2 | `f5865f848b70` | 1/1 | 0/0 |

Six GitHub release URLs redirected to `release-assets.githubusercontent.com`. The two GitHub tag archives, `zstd` and `brotli`, redirected to `codeload.github.com`. Both GitLab archives, `ca-certificates` and `apk-tools`, are generated on demand. All still matched the declared sha512. Final URLs are recorded per file.

## License files

After a sha512 match, 18 upstream tar archives were read in memory. Only regular members at depth ≤ 3 whose names match `COPYING|LICENSE|LICENCE|COPYRIGHT|NOTICE|AUTHORS` were written. Absolute paths, `..` components, links, files over 256 KiB and output resolving outside `licenses/` were rejected. Nothing was extracted to disk otherwise. The run wrote 38 files totalling 200,656 bytes and rejected or skipped none. The `ca-certificates` archive has no matching license file. `zstd/build/LICENSE` is empty. These files show only what the archives contain. They make no claim about legal clearance, license identification, notice completeness or binary reproducibility.

## Remaining gaps

- The exact declared source and patch inputs are verified, but the installed APK binaries were not rebuilt from them. This review does not show source-to-binary correspondence.
- Build dependencies, the abuild/toolchain versions and builder environment were not retrieved.
- Aports-local files were checked against the GitHub mirror at the exact commit, not against Alpine GitLab. GitLab raw returned HTTP 418 earlier.
- `alpine-base` declares no sources; its content comes from the APKBUILD itself.
- No license texts were reviewed.

The archive cap is not a blocker. The run used 21% of 250 MiB. Free disk was 8.25 GiB at the start and 6.90 GiB at the end. The per-file floor check never triggered.

## Evidence

`.local/opus-resume/alpine-source-payloads/`: `inventory.tsv` (expanded URL and declared sha512 per file), `manifest.tsv`/`.json` (status, final URL, bytes, sha512, sha256, match, path, error, license actions), `fetch.log`, `inventory.log`, `fetch.py`, `downloads/`, `licenses/`. `manifest.json` SHA-256 `e53727dabbf3ff6f2cda493bdd096efa0b402a14e2bbf72052b73b3e8663be18`.
