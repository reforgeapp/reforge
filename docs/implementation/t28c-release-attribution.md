# T28c release attribution evidence map

Evidence collected 2026-09-23. This records local provenance and exact gaps for the images listed in [the T28c notice review](t28c-notices-review.md). It is not a legal determination.

## Inputs and retained artifacts

| Scope | Evidence | What it establishes | Open limit |
| --- | --- | --- | --- |
| Control APK database | `.local/t28c-attribution/control.apk-packages.tsv` | 16 installed package records from image `sha256:fdb90334236c0783b9a211230a327294e8e49598f4f40a7b600d2f625ba46f18` | No package source texts retained in image; immutable build-time APK indexes/source archives not captured |
| Runner APK database | `.local/t28c-attribution/runner.apk-packages.tsv` | 28 installed package records from image `sha256:aba618640269d1b76983ccdb4d2958db967c1a30d56849ed32f7bb8d875aeed2` | Same |
| Docs APK database | `.local/t28c-attribution/docs.apk-packages.tsv` | 68 installed package records from image `sha256:d1ee1b08837c5f9e8e6ded7a66f5b9aa34d54d7c37dbcd9ee1e414d899660d6f` | Same; five nginx-related copyright paths are present, their image text was not copied into this evidence set |
| PostgreSQL APK database | `.local/t28c-attribution/postgres.apk-packages.tsv` | 53 records from image `sha256:c293117fcecda7344b5480222e813b9f673d7abd69b1dd95eff239b768b04f59` | No license/copyright paths found; `.postgresql-rundeps` is synthetic metadata without a license tag |
| Alpine package retrieval sample | `.local/t28c-attribution/apk-download/` | Current control Alpine v3.21 index metadata and exact `ca-certificates` APK fetch; archive SHA-256 is separate from installed APK DB `C:` | Only one package sampled; current branch index is not proof of the historical build-time index for all images |
| Docs asset hashes | `.local/t28c-attribution/docs-asset-source-hashes.tsv` | All 43 delivered site assets byte-match files in `mkdocs-material==9.7.0` | Source mapping does not itself place notice text in the final image |
| Embedded docs JS packages | `.local/t28c-attribution/embedded-npm-report.json`, `npm-embedded/` | Seven embedded package tarballs match upstream Material 9.7.0 lock integrity and include root text files with hashes | Applicability/content inclusion in shipped product notice file still needs reconciliation |
| Material upstream graph | `.local/t28c-attribution/material-source/package-lock.json` | Exact upstream tag commit `3308731f1dce2e72809a2167d900b3381ca8d0d1`; package lock SHA-256 `f0672a19748d0c967f86a9ee80855415128631cb0935808c69a31e3bbda40b0c` | The theme's npm lock describes bundled JS sources, not the full site's notice obligations |
| Icon pack scan | `.local/t28c-attribution/docs-icon-pack-scan.json` | Six used inline icons match the Material icon pack; Python source distribution has its license text | Scan matches SVG path strings in HTML; broader icon/resource use needs review |
| Current app frontend build | `.local/t28c-attribution/web-sourcemap-dist/` | Current source build sourcemap resolves to packages in `web/package-lock.json` | Bundle hash differs from inspected control image; not exact provenance for image bytes |
| Image frontend bytes | `.local/t28c-attribution/control-bundle-sha256.txt` | Exact JS, CSS and HTML hashes in the locally inspected control image | Image has no sourcemaps and exact source snapshot used to build it is unavailable |
| Python metadata anomaly | `.local/t28c-python-lock/mergedeep-1.3.4-LICENSE.txt` and wheelhouse/source archive | Exact sdist and MIT license text hashes for `mergedeep==1.3.4` | Installed metadata still says `UNKNOWN`; do not rewrite that metadata as though upstream had declared MIT |

The ignored evidence is local review material, not yet a generated, release-attached SBOM or notice bundle. Image IDs are local Docker image IDs, not published registry digests.

## Exact attribution gaps

- For control, runner, docs, and PostgreSQL, installed APK metadata records names, versions, architectures, checksums, package build commits, and license tags. Full source package provenance and complete applicable text have not been captured for every record. `L:` is not a substitute for source text. Docker builds use mutable Alpine branch repository URLs; current indexes cannot be assumed to equal build-time indexes.
- The docs image retains five nginx and module `COPYRIGHT` files under `/usr/share/licenses`; four share SHA-256 `4f72e2bccf0bcd839dd447ab6f359336f428caa1987a9afec2bc9ade5711658a`, while njs is `bf0746e75035cc2b9c3346feb08fd7aa414c088719c00e76734c6d7fe9c0647d`. Match these image texts to corresponding package source before release attribution is called complete.
- The theme's 43 site assets all match the locked Python wheel. Its minified JS sourcemaps identify seven npm package sources; exact upstream npm lock integrity and root text hashes are collected. The existing project notice artifact must be checked for every applicable text, and generated-site icon attribution must be included where required.
- Current inspected control-image JavaScript differs from the current-source frontend build. The production npm lock has 15 runtime packages, while the known 28 package-root text gaps are dev-only. Those dev-only files are not by themselves runtime-image distribution blockers because the builder's installed tree is not copied. Exact provenance of the inspected final JS bundle is still open; verify from a clean build of the exact source revision before release.
- `mergedeep` source contains a license text despite metadata `UNKNOWN`; preserve both facts and their hashes. Do not infer package metadata solely from the text.

## Safe notice generation sequence

1. Build and record each release image by immutable OCI manifest digest and platform. Export an SPDX or CycloneDX SBOM from each final image; keep the raw image digests and SBOM hashes alongside it.
2. For every final APK record, preserve the exact repository URL, signed `APKINDEX` and signature, package row, `.apk` archive, archive hash, and installed database record. Resolve the archive's source package/APKBUILD and exact source revision/checksum. If the old index or source is unavailable, record that package as unresolved; do not substitute current branch metadata.
3. Extract package-provided `LICENSE`, `COPYING`, `NOTICE`, and copyright texts from the exact source package or installed image. Keep bytes unchanged, record source package/file and SHA-256, and deduplicate only identical text hashes while retaining every package-to-text mapping. Do not translate Alpine license tags to SPDX IDs by string replacement.
4. For language/build dependencies, use lockfile artifact hashes and exact source distributions. Trace generated assets in the final image back to those artifacts; include their applicable text and attribution records. Keep builder-only packages scoped separately from packages or code actually shipped.
5. Generate the human-readable notice document deterministically from the package-to-text manifest. Fail the generator on unknown package identity, missing source hash, or missing required text; emit a machine-readable unresolved list rather than inventing a license or silently omitting a package.
6. Compare the generated output against each final image digest in release CI. Store command versions, platform, inputs, output hashes and unresolved list as release artifacts. Require separate owner/legal review for applicability and release disposition.

A suitable future generator should consume explicit SBOM/package/source evidence and exact text files; it must not download whatever is newest from mutable package branches during notice generation. No generator was added in this evidence-only slice.
