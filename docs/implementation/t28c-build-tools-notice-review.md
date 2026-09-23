# T28c build-tool notice review

Reviewed 2026-09-23. `deploy/docs/requirements.lock` SHA-256: `68426162a84903217ba12250e3b8a267c9798e4408fbf0c9dc9a444616756137`.

## Python build tools

Downloaded `setuptools==84.0.0` and `wheel==0.48.0` with `pip download --require-hashes` from the lock. Both archive SHA-256 values matched the lock:

| Archive | SHA-256 | Notice records |
| --- | --- | ---: |
| `setuptools-84.0.0-py3-none-any.whl` | `51a52592b3b99e102b609654876bd65f19f999935166d1352678931132b0c670` | 17 |
| `wheel-0.48.0-py3-none-any.whl` | `3217dcc807155e45db462d7ef2431f5ddda0d7273b700d05a67b271ceb1287ab` | 1 |

The 18 notice records preserve exact wheel-member bytes, including setuptools' bundled vendor notices. `manifest.json` records member paths and archive hashes. The existing generator verified notice hashes and produced identical files on two runs: `dependencies.md` SHA-256 `a65eeef20220cba6f68d5931f4b86a8ec38f55687b6e9d2c276b2f14cde08cf4`; `third-party-notices.txt` SHA-256 `49413aee16d4d7983871e92d592a5047ae2fedcf3b593fe10898bcb4354b3af7`.

## Alpine package evidence

Inspected local images `reforge-control:t28c-2026-09-23` (`sha256:fdb90334236c0783b9a211230a327294e8e49598f4f40a7b600d2f625ba46f18`), `reforge-runner:t28c-2026-09-23` (`sha256:aba618640269d1b76983ccdb4d2958db967c1a30d56849ed32f7bb8d875aeed2`), and `reforge-docs:t28c-python-lock` (`sha256:d1ee1b08837c5f9e8e6ded7a66f5b9aa34d54d7c37dbcd9ee1e414d899660d6f`). All use Alpine v3.21 x86_64. Their `/var/cache/apk` directories contain no package archives.

Downloaded current official `v3.21/main` and `v3.21/community` indexes. `apk update` accepted their signatures using the control image's trusted Alpine keys. Comparing installed records by package, version, architecture and APK checksum found exact current-index entries for control `16/16`, runner `28/28`, and docs `41/68`. Current indexes therefore identify current public archives for all control and runner packages and 41 docs packages. No APK archives were downloaded.

The docs image has 27 package versions absent from current indexes. Its configured repositories point to rolling `v3.21`; version-specific `v3.21.3` index URLs return HTTP 404. No local cached APK archives or build-time APK indexes were found. Retain signed indexes and exact APK archives for each final image build to close those records. Current index metadata does not recover missing historical package inputs.

No delivery-path decision or legal review was made. These records do not establish complete OS source notices or legal clearance.
