# T19 toolchain images

`scripts/build-runner-images.py` prepares one operator-selected Go, JavaScript, or Python runtime without Docker, network access, package installation, or cluster changes.

The command requires `--stack` and an absolute `--output` path that does not already exist. It validates the selected executable, copies its ELF interpreter, and resolves executable dependencies with that trusted interpreter's `--list` output. Shared objects use `ldd`; Python extension dependencies use the Python interpreter when available. Missing dependency errors are fatal. Homebrew glibc libraries are copied at both their canonical reported paths and the interpreter-prefix aliases required by the loader. The packer rejects toolchain directory symlinks, dereferences file symlinks, removes bytecode/cache/test/documentation/README files where safe, and creates the exact runtime mount directories plus an empty regular `opt/reforge/tool` placeholder. Only the newly created partial output is removed on failure; existing paths are rejected before creation.

Go uses its installed `GOROOT`; Python uses its installed standard library; Node uses its installed binary. Output reports `imagepath` and `toolchainversion`. Operators must run `sandbox.ImageDigest` afterward, register the resulting digest, and verify the image in a real gVisor fixture. The packer does not claim sandbox execution or dependency installation.

The integration fixture built fresh images and verified the full baseline-failure/source-only-patch/pass flow under gVisor for Go, Node, and Python. The loader check exposed host `ldd` selecting incompatible libraries; trusted interpreter resolution plus the glibc aliases fixed Node and Python startup without widening runtime mounts.
