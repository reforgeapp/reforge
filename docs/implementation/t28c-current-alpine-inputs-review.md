# T28c current Alpine inputs review

**Scope:** Alpine packages in current control and runner images only. No Dockerfile, notice, or product changes made.

## Images and package records

Read `/lib/apk/db/installed` directly from local amd64 images `reforge-t28c-current-control:18c8045` (`sha256:80f66adaf351338158c17d706cfdda3a870c6a71f5aba6e0b3c98f5f7e31bf25`) and `reforge-t28c-current-runner:18c8045` (`sha256:53438610f14b72d13bb4ba525c1dacf0a46220a6506340d0f221ee07b3627af9`). Final frozen images at `675b0fd` are control `sha256:59819fbd3a751b824117728157934a1b75af1228901848d1c9794c5499f17da0` and runner `sha256:53438610f14b72d13bb4ba525c1dacf0a46220a6506340d0f221ee07b3627af9`. These are local image IDs, not registry digests. Full installed APK records, including package checksum `C`, source commit `c`, origin and license tag, match between 18c8045 and 675b0fd: control 16/16; runner 28/28. Installed architecture fields are `x86_64`; all package checksum/source records match the official v3.21 main index.

Exact tuple deduplication by installed name/version/architecture/checksum yields 28 unique APKs across the two images; 21 unique `(source commit, source package)` references are recorded in `packages-dedup.tsv`.

## Official archive and signature checks

Fetched each exact installed `name-version.apk` once from Alpine’s official `v3.21/main/x86_64` CDN path. All 28 returned HTTP 200; no newer package was substituted. Downloaded APKs plus main/community indexes total 11,955,549 bytes, below the 50 MiB limit. Per-URL status, exact bytes, SHA-256 and path are in `official-downloads.tsv` and `.json`.

The official main index SHA-256 is `cd3111e0e4afc3bf129646882012da6bd236ad59f9362ec722f0a4ce8eb0a4c7`, matching the previously retained v3.21.8 index. The official community index hash is `bb0154701068ff0b08d5e410dc563620770b6075990afc3bd7ed620cf5dea644`. Offline `apk update` accepted both downloaded indexes using the final control image’s trusted Alpine keys. The signed main index matches all 28 installed package checksums and source commit fields.

`apk verify` returned `0 - OK` for all 28 downloaded archives against trusted keys in each final control and runner image. A fresh index generated from those verified archives reproduced installed `C` for all 28/28 packages. Six noarch archive records report installed architecture `x86_64` in the installed database; their checksums and name/version still match. Per-package archive/index comparisons and both image key fingerprints are retained under `.local/opus-resume/alpine-current/`.

The official [APK spec](https://wiki.alpinelinux.org/wiki/Apk_spec) defines the index checksum field; exact official package paths are beneath the [Alpine v3.21 main x86_64 repository](https://dl-cdn.alpinelinux.org/alpine/v3.21/main/x86_64/).

## Remaining provenance gaps

The installed records provide 21 exact aports commit references. The first exact raw GitLab URL (`pcre2`, commit `0826e91f0c743946bdc6ffa51fe90479bf4a83da`) returned HTTP 418. The official [Alpine aports GitHub mirror](https://github.com/alpinelinux/aports) identifies itself as a mirror and links back to Alpine's GitLab repository. Fetching only the 21 exact `raw.githubusercontent.com/alpinelinux/aports/<commit>/main/<origin>/APKBUILD` paths returned HTTP 200 for all 21. Literal `pkgname`, `pkgver`, and `pkgrel` fields matched each installed origin and version (21/21); no APKBUILD was executed. Exact URLs, response byte counts, SHA-256 hashes, parsed fields, and retained bytes are in `github-mirror-retrieval.json`, `.tsv`, and `apbuilds/`.

The first mirror pass retained the same 21 bodies but a reporting-script error prevented its status manifest from being written; the exact bounded pass was repeated to retain that manifest. Together, both passes transferred 190,354 bytes, below the 5 MiB task cap. The earlier GitLab 418 and URL remain recorded in `source-retrieval.json`.

The APKBUILDs name upstream source archives and some package patches, but none of those archive or patch payloads were fetched or matched against the installed binaries. No package-owned license texts were extracted or reviewed, and no legal clearance is claimed. This verifies exact installed-package archives, signatures, checksums, source commit fields, and matching APKBUILD version declarations for these control and runner image IDs; it does not establish build-time index provenance, source/patch correspondence, or notice completeness. The earlier docs-image findings remain out of scope.
