# T28c notice closure review

Reviewed 2026-09-23. Generator verifies each committed notice text against SHA-256 in `notice-sources/manifest.json`, then writes source identity and text hash beside the exact text in `third-party-notices.txt`. Source texts are snapshots; no license family or legal clearance is inferred.

## Added evidence

| Source | Text records | Provenance |
| --- | ---: | --- |
| Embedded docs npm packages | 8 | Seven locked packages; exact tarball integrity from Material 9.7.0 lock and archive SHA-256 retained in manifest. `tslib` contributes both copyright notice and license. |
| MkDocs Material and Material icons | 2 | Exact `mkdocs-material==9.7.0` source archive SHA-256; source texts copied from archive. |
| Locked docs Python distributions | 33 | Texts copied from locally built docs builder image `sha256:c0b19514dda3599b7d85136373cb498398d8a0feda716de45a70b039bc281b61`; lock SHA-256 `68426162a84903217ba12250e3b8a267c9798e4408fbf0c9dc9a444616756137`. |
| Docs nginx image copyright files | 5 | Exact files copied from `/usr/share/licenses` in docs image `sha256:d1ee1b08837c5f9e8e6ded7a66f5b9aa34d54d7c37dbcd9ee1e414d899660d6f`. |

The builder image contains notice texts for 29 locked Python distributions, producing 33 files; `setuptools==84.0.0` and `wheel==0.48.0` lock entries have no matching installed `.dist-info` notice text in that image. `mergedeep==1.3.4` remains package metadata `UNKNOWN`; exact bundled MIT text is included without changing that metadata claim. Python lock permits platform-specific archives, so selected wheel archive identities are not established by the installed image alone.

## Remaining evidence

The generated notice file remains repository-side evidence only: `deploy/control/Dockerfile`, `deploy/runner/Dockerfile`, and `deploy/docs/Dockerfile` do not copy `docs/implementation/third-party-notices.txt` into their final images. No delivered-image notice compliance is established; decide the supported notice delivery path after release-owner/legal review.

- Control and runner Alpine packages and PostgreSQL Alpine packages: final image records exist, but exact build-time signed APK indexes, matching APK archives/source trees, and complete notice texts are not retained. Docs image now contributes its five bundled nginx copyright files; that does not cover its Alpine dependency closure. Current-branch package metadata or a replacement archive cannot prove historical build inputs.
- `setuptools` and `wheel`: obtain source distributions or wheels matching the accepted lock hashes, verify archive identity, and include their exact root notice texts if applicable.
- Final control image: refresh clean image after the integrated release source revision and record image digest plus exact emitted bundle hashes. Existing stale local image does not establish final bundle provenance.
- Arm64 images, full source-package attribution, and human/legal review remain open. Preserve exact OCI digests, APK index/signature, archive checksums and source revision for each platform before making release claims.

No publication or legal review was performed. T28c remains incomplete until exact final image inputs and outstanding source texts are reconciled and an authorized owner completes release review.
