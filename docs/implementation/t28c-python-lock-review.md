# T28c docs Python lock review

Reviewed on 2026-09-23. `deploy/docs/requirements.lock` pins the 29 distributions resolved in the prior MkDocs build, excluding `pip` itself, plus the two hash-checked build tools used for source fallback. Every package has an exact version and SHA-256 artifact hash. Lock contains amd64 and arm64 hashes for platform wheels; watchdog has its amd64 wheel hash and source archive hash. The Python base image digest pins the pip installer version. The builder rejects architectures other than x86_64 and aarch64.

The Dockerfile installs the first two lock entries in a separate hash-checked step so the pinned setuptools and wheel exist before pip builds watchdog's source archive with `--no-build-isolation`. It then installs the complete hash-required lock and runs strict MkDocs. This avoids downloading unpinned PEP 517 build dependencies during source fallback.

`mergedeep==1.3.4` distribution metadata still says `License: UNKNOWN`. Its exact source archive, SHA-256 `0096d52e9dad9939c3d975a774666af186eda617e6ca84df4c94dec30004f2a8`, contains a `LICENSE` file with this text (SHA-256 `11592b9d56693c987da849c075d636690916d01a47ca234a14bbcf0fe176b37`):

> The MIT License (MIT)
>
> Copyright (c) 2019 Travis Clarke <travis.m.clarke@gmail.com> (https://www.travismclarke.com/)
>
> Permission is hereby granted, free of charge, to any person obtaining a copy
> of this software and associated documentation files (the "Software"), to deal
> in the Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
> copies of the Software, and to permit persons to whom the Software is
> furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in
> all copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
> FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
> AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
> LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
> THE SOFTWARE.

## Verification

- `docker build --pull=false -t reforge-docs:t28c-python-lock -f Dockerfile .` from `deploy/docs` completed on local linux/amd64. `mkdocs build --strict --site-dir /site` passed inside that build.
- `pip check` reported no broken requirements. Installed builder versions matched all 31 lock entries; only base-image `pip` was additional. `mkdocs --version` reported 1.6.1.
- Final image ID: `sha256:d1ee1b08837c5f9e8e6ded7a66f5b9aa34d54d7c37dbcd9ee1e414d899660d6f` (`linux/amd64`). HTTP smoke returned 200 for `/docs/`, versioned home, install, troubleshooting, search index, and `/docs/versions.json`. Search data contained budget and troubleshooting content. `/opt/docs-site/search/search_index.json` exists in the image; no Python runtime or Python library directory was copied into it.
- Pip's offline `manylinux_2_28_aarch64` resolver selected hash-matching arm64 wheels for every lock entry except watchdog. The pinned Python OCI index includes child manifest `sha256:67994a05c712036dbfc4385b4bceafc0ce20df950f54b9ea355582c153bf6157` for arm64. Watchdog's pinned source archive built locally into `watchdog-6.0.0-py3-none-any.whl`; its Linux setup defines no architecture-specific extension.
- A full arm64 Docker build could not run here: the host lacks arm64 emulation and returned `/bin/sh: exec format error`. Therefore strict image build is locally certified for amd64 only; arm64 artifact selection is hash-verified, but its container build remains unverified. Docker also printed its missing Buildx metadata-probe warning before using the legacy builder successfully.

Local ignored evidence: `.local/t28c-python-lock/locked-build-distributions.json`, `tool-versions.txt`, `final-image.txt`, `http-smoke.json`, `arm64-wheel-check/`, `arm64-wheel-check.lock`, and `mergedeep-1.3.4-LICENSE.txt`. Downloaded package artifacts are retained in `.local/t28c-python-lock/wheelhouse/`, `aarch64/`, and `source/` for this local review.

The lock resolves dependency versions and archive identity; it does not copy Python package notice texts into `third-party-notices.txt`. The generated docs site contains theme assets from the Python build graph, and this review did not map every shipped asset to source or confirm its attribution in the final docs image. Final docs-image notice and asset attribution remain release evidence gaps. No legal compatibility or clearance conclusion is made.
