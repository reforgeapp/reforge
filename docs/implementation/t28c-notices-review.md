# T28c notice evidence review

Reviewed 2026-09-23 against locally available final images, docs build and frontend build. This is evidence collection, not legal advice, compatibility analysis, or release approval.

## Final image evidence

| Image | Local image ID | Platform | APK database records | Base/package evidence |
| --- | --- | --- | ---: | --- |
| `reforge-control:t28c-2026-09-23` | `sha256:fdb90334236c0783b9a211230a327294e8e49598f4f40a7b600d2f625ba46f18` | linux/amd64 | 16 | Alpine 3.21.8 |
| `reforge-runner:t28c-2026-09-23` | `sha256:aba618640269d1b76983ccdb4d2958db967c1a30d56849ed32f7bb8d875aeed2` | linux/amd64 | 28 | Alpine 3.21.8 |
| `reforge-docs:t28c-python-lock` | `sha256:d1ee1b08837c5f9e8e6ded7a66f5b9aa34d54d7c37dbcd9ee1e414d899660d6f` | linux/amd64 | 68 | Alpine 3.21.3, nginx 1.27.5 |
| `postgres:18.6-alpine` | `sha256:c293117fcecda7344b5480222e813b9f673d7abd69b1dd95eff239b768b04f59` | linux/amd64 | 53 | Alpine 3.24.2 |

Exact installed package records (name, version, architecture, APK database checksum, build commit and license tag) are retained in ignored evidence files `.local/t28c-attribution/{control,runner,docs,postgres}.apk-packages.tsv`. PostgreSQL's database includes a synthetic `.postgresql-rundeps` entry with no license tag. Searches found no license/copyright basename files in control, runner or PostgreSQL. Docs retains nginx and nginx-module copyright files under `/usr/share/licenses`; their exact paths were collected in `.local/t28c-attribution/reforge-docs_t28c-python-lock.copyright-paths.txt`. The five image files reduce to two SHA-256 values: `4f72e2bccf0bcd839dd447ab6f359336f428caa1987a9afec2bc9ade5711658a` (nginx and three modules) and `bf0746e75035cc2b9c3346feb08fd7aa414c088719c00e76734c6d7fe9c0647d` (njs module). Exact copies used for hashing are in ignored `/tmp/t28c-all-licenses/` for this local session.

The Alpine `L:` value is package metadata, not a copy of required license/notice text and not independently classified here. For one control-image package, `ca-certificates`, local `apk update` and `apk fetch --from repositories ca-certificates` yielded an APK whose version, architecture, APK database `C:` string and build commit could be paired with the exact current signed-index record; archive SHA-256 is recorded separately. This demonstrates a route to evidence for that package only. The Dockerfiles use versioned Alpine branches rather than immutable repository snapshots. It does not establish that all installed APK archives/source trees and required texts for these final images remain retrievable or match the build-time indexes. No blanket license mapping or legal conclusion follows.

## Locked docs build and delivered site

The current docs Dockerfile uses a hash-checked Python requirements lock, installs build tools in a separate hash-checked step, runs `mkdocs build --strict`, and copies only `/site` into the nginx image. The final image contains no Python virtual environment. The prior review's image IDs and Python-lock statements are superseded by this locked build; see [the Python lock review](t28c-python-lock-review.md).

The final docs image contains 43 files under `/opt/docs-site/assets`. Every one byte-matches a file from the exact `mkdocs-material==9.7.0` distribution; path and SHA-256 pairs are in ignored `.local/t28c-attribution/docs-asset-source-hashes.tsv`. This proves asset origin from that distribution, not that the current site's embedded third-party components have all required notices in the delivered site.

Material 9.7.0's exact upstream tag resolves to commit `3308731f1dce2e72809a2167d900b3381ca8d0d1`; its `package-lock.json` (SHA-256 `f0672a19748d0c967f86a9ee80855415128631cb0935808c69a31e3bbda40b0c`) lists the packages embedded in its bundled asset source maps. The seven identified packages are `clipboard@2.0.11`, `escape-html@1.0.3`, `focus-visible@5.2.1`, `lunr@2.3.9`, `rxjs@7.8.2`, `tslib@2.7.0`, and `material-design-color@2.3.2`. Exact npm tarballs were checked against the upstream lock integrity values. Their root license/copyright texts and hashes are in `.local/t28c-attribution/embedded-npm-report.json` and `npm-embedded/`. None of these seven package text files appears verbatim in the current `docs/implementation/third-party-notices.txt` (checked by exact text comparison after trimming outer whitespace). Their presence inside source maps does not establish notice fulfilment.

The generated docs HTML uses six inline icons matching the Material icon pack: `arrow-up`, `library`, `menu`, `arrow-left`, `magnify` and `close`. That pack's license text is available in the exact Python distribution source, but the final image only contains its rendered glyph paths, not that text. Icon scan evidence is `.local/t28c-attribution/docs-icon-pack-scan.json`. Other configured icon packs had no matching inline icon paths in that scan; treat this as a focused check, not exhaustive proof that no other icon glyph is used.

`mergedeep==1.3.4` reports `UNKNOWN` in package metadata. Its exact sdist was recovered from the locked build evidence and its MIT license text extracted and hashed (`11592b9d56693c987da849c075d636690916d01a47ca234a14bbcf0fe176b37`); the archive hash and text are in `.local/t28c-python-lock/`. The metadata anomaly remains to be documented with that source text; it should not be silently rewritten as authoritative package metadata.

## Frontend bundle and npm development packages

The inspected control image contains `/app/web/dist/assets/index-CJN-NXVT.js` (SHA-256 `a0cfa8b5498243050b77aef60a4f2882988a6da7c99ef1ed0b61789c578367b9`), CSS `index-Bd1oeqz3.css` (SHA-256 `57d8f580a6a1e1ca93749026d0cb102b63d0cd9e972142c5c87911c0dfc0ae37`), and `index.html` (SHA-256 `ab28722673ad85d35d1ebb34e3a55ff6834b967a2f7b5ec37f14fa0affaedb09`). A fresh local build from the current source and lockfile produced a different bundle (`index-Crtub4So.js`); its sourcemap identifies the locked runtime packages, but it is not the bundle in the inspected image. Because final bundle source maps are absent and the image cannot be rebuilt from the exact source snapshot used for that image, exact image-bundle-to-package provenance is unresolved.

The inventory's 28 npm root-text gaps are all development-only lockfile entries, not installed packages copied into the control runtime image. On current Dockerfile evidence, these missing texts are not a blocker for distributing the runtime image as such; they remain a build-input tracking gap if builder layers/cache are distributed, and must be revisited if any development package code is incorporated in shipped output.

## Disposition and evidence needed

T28c notice evidence remains incomplete. Before claiming a complete release notice set, generate it from exact final image digests and build inputs; collect exact source/package records and full texts for every shipped base-image and OS package; include the docs Python and generated-site assets' applicable texts; verify the control bundle against the source/lock used for the final image; and reconcile the existing notice file against these sources. See [the release attribution evidence map](t28c-release-attribution.md) for artifact paths and a reproducible collection sequence. No legal clearance, license compatibility, or blanket telemetry conclusion is made here.
