# T28c docs image inputs review

**Scope:** the 27 docs-image APK rows left unmatched by [t28c-alpine-provenance-review.md](t28c-alpine-provenance-review.md), 5 of which (nginx) lack source commits. No Dockerfile, notice, base image or product change was made.

## Image read

Final `reforge-t28c-final-docs:675b0fd` is image ID `sha256:7ad3cbc38c7fbe740c60f3969693368781d8ad6052f3f0a8f5c2e09046efc69c`, a local ID with no registry digest. `/lib/apk/db/installed` has 68 records, and all 68 match the prior review rows. The same 27 remain unmatched.

The runtime base `nginx:1.27-alpine@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10` was inspected read-only. Its local config ID is `sha256:6769dc3a…`. Config history shows:

- `ADD alpine-minirootfs-3.21.3-x86_64.tar.gz`
- `apk add -X https://nginx.org/packages/mainline/alpine/v3.21/main` of nginx `1.27.5-r1`, modules `1.27.5-r1` and njs `1.27.5.0.8.10-r1`
- `apk add curl ca-certificates` from the mutable Alpine v3.21 repositories
- pkg-oss `1.27.5-1` sha512 `c773d98b…ac1745`

## Binary archives

| Evidence | Rows | Result |
| --- | ---: | --- |
| Official `alpine-minirootfs-3.21.3-x86_64.tar.gz` | 10 | Official `.sha256`/`.sha512` matched. GPG signature was good for key `0482D84022F52DF1C4E7CD43293ACD0907D9495A`, fetched from `alpinelinux.org/keys/ncopa.asc`; web-of-trust was not established. The tarball's installed DB is identical (version, `C`, `c`) for `alpine-release`, `busybox`, `busybox-binsh`, `ca-certificates-bundle`, `libcrypto3`, `libssl3`, `musl`, `musl-utils`, `ssl_client` and `zlib`. Image layer 0 matched the tarball for all 520 entries, with 0 differences. |
| `alpine-extended-3.21.3-x86_64.iso` `/apks/x86_64`, HTTP range reads | 12 | The 10 base rows plus `ca-certificates` and `nghttp2-libs`. |
| `alpine-extended-3.21.4-x86_64.iso` `/apks/x86_64`, HTTP range reads | 6 | `c-ares`, `curl`, `libcurl`, `libexpat`, `tzdata`, `xz-libs`. |
| nginx.org `packages/mainline/alpine/v3.21/main/x86_64` | 5 | All 5 exact APKs and `APKINDEX.tar.gz` returned HTTP 200. |
| Not recovered | 4 | `libpng 1.6.47-r0`, `libxml2 2.13.4-r5`, `libxpm 3.5.17-r0`, `tiff 4.7.0-r0`. |

All 18 ISO APKs, both ISO indexes, the 5 nginx APKs and the nginx.org index returned `apk verify` `0 - OK`. This ran offline in the final docs image with `--network none`, using its trusted keys. The downloaded `nginx_signing.rsa.pub` byte-matches the image key. `apk index` over the recovered archives reproduced installed `C` and `c` for 18/18 Alpine rows and `C` for 5/5 nginx rows. The signed nginx.org index also matches all 5 `C` values and build times. Full ISOs were not downloaded or hashed; trust rests on per-APK and index signatures.

Exact `v3.21/main/x86_64/<name>-<version>.apk` CDN paths returned HTTP 404 for all 22 Alpine rows. The 4 unrecovered packages are absent from both extended ISO indexes. Standard/virt ISOs, other point releases and other channels were not tried.

`apk audit --system`, run offline as root in the final image, reports only `X etc/nginx/conf.d/default.conf`, which the Reforge Dockerfile deletes. All other installed files match their DB checksums, including those of the 4 unrecovered packages.

## Source inputs

| Group | Declared files | sha512 match | Notes |
| --- | ---: | ---: | --- |
| 16 aports commits (GitHub mirror APKBUILD, exact commit) | 110 | 110 | APKBUILD `pkgver`/`pkgrel` matches the installed version for 16/16. `alpine-base` declares no sources. 82 files (busybox 68, musl 14) are reused read-only from `alpine-source-payloads` after an identical sha512 re-hash. 28 were newly fetched. |
| nginx pkg-oss `1.27.5-1` | 1 | 1 | Matches the sha512 in the official image history. The archive commit comment is `45a3cf7729d8fd7bc52c81e4c73b1a5abc5260fa`. |
| nginx/njs/QuickJS per pkg-oss `SHA512SUMS` | 3 | 3 | `nginx-1.27.5.tar.gz`, `njs-0.8.10.tar.gz` and `quickjs-2024.07.24-6e2e68fd….tar.gz` came from pkg-oss's primary cache, `packages.nginx.org/contrib`. |

The declared SourceForge URL for `expat-2.7.0.tar.bz2` returned HTTP 404. Alpine's builder distfiles cache, `distfiles.alpinelinux.org/distfiles/v3.21/`, served the exact filename, and it matched the declared sha512. For `libxml2`, `${pkgver%.*}` was expanded manually to `2.13`. No APKBUILD or downloaded script was executed.

## Notices

- **Alpine rows:** 29 license files (128,818 bytes) were extracted from the verified archives. Extraction was limited to depth ≤ 3, regular files only, 256 KiB per file, and rejected traversal and links. Busybox and musl texts come from the reused `alpine-source-payloads/licenses/` archives, which are identical. `ca-certificates` and `alpine-base` have no license file in their inputs; only metadata license tags were found in these inputs; that does not establish applicable notice obligations.
- **nginx rows:** all 5 installed `/usr/share/licenses/*/COPYRIGHT` files, including those inside the APKs, byte-match pkg-oss `docs/<package>.copyright`. Those files are `4f72e2bc…` for nginx and the geoip/image-filter/xslt modules, and `bf0746e7…` for njs. They differ from the upstream `nginx-1.27.5/LICENSE` and `njs-0.8.10/LICENSE`, which were retained.
- **QuickJS:** the njs `.so` and `usr/bin/njs` binaries contain `QuickJS` strings, and pkg-oss builds njs against QuickJS. The installed njs COPYRIGHT does not mention QuickJS. The QuickJS `LICENSE` was retained from the verified archive.

This is technical mapping only. No legal clearance, notice completeness or binary reproducibility is claimed.

## Result

| Class | Rows |
| --- | ---: |
| Exact signed binary + exact-commit/pinned source verified | 23 |
| Exact source verified, binary archive not recovered | 4 |
| Unknown source | 0 |

Per-row mapping: `.local/opus-resume/docs-image-provenance/package-map.tsv` (SHA-256 `5bb2e60d…`).

The 4 unrecovered packages are pulled in only through `nginx-module-image-filter` (libgd → `libpng`, `libxpm`, `tiff`) and `nginx-module-xslt`/`nginx-module-njs` (`libxslt` → `libxml2`). `deploy/docs/nginx.conf` loads no dynamic modules. `curl`, `libcurl`, `c-ares` and `nghttp2-libs` are needed only by `curl`. No docs healthcheck or entrypoint uses `curl`.

## Remaining actions

1. Optionally extend the range probe to the standard/virt 3.21.3/3.21.4 ISOs and the 3.21.5+ extended ISO indexes for the 4 packages. They were not tried, and their availability is unknown.
2. Otherwise, correct the packaging: pin a runtime whose install step contains no dynamic modules and no `curl`. That removes all 4 unrecovered packages and the QuickJS notice gap. Candidates are nginx `-alpine-slim` pinned by digest, or an Alpine base plus `nginx`; verify the chosen image's contents before adopting it. During the build, export the exact `.apk` archives and signed indexes (for example, an `apk fetch` evidence stage), then repeat the checks above.
3. After that rebuild, regenerate the docs-image notices from the mapped texts: nginx pkg-oss COPYRIGHT plus the per-origin source license files. Add QuickJS only if njs stays.
4. Release-owner/legal review.

## Budget and evidence

New downloads totalled 53,395,290 bytes (35.6% of 150 MiB): 83 whole-file requests (60 HTTP 200, 23 HTTP 404) plus 28 ISO range reads (4,770,015 bytes). Free disk was 5.07 GiB at the start and 3.68 GiB at the end. Other workers were active at the same time, and the per-file 3 GiB + 256 MiB floor never triggered.

Evidence is under `.local/opus-resume/docs-image-provenance/`: `manifest.tsv`, `phase*.log`, `iso-range-requests.tsv`, `downloads/`, `image/` (installed DB, keys, notices, inspect/history), `verify/` (minirootfs/layer, apk verify/index/audit, reverse deps, source inventory, reuse re-hash), `licenses/`, `licenses-manifest.json`, `package-map.tsv` and the scripts `fetch.py`, `iso_range.py` and `licenses.py`.
