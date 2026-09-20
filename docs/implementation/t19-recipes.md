# T19 validation recipes

`recipes.Build` creates deterministic v1 baseline recipes for Go, JavaScript, and Python from an in-memory repository snapshot. It performs no execution, dependency installation, or network access.

Go runs `go test -json -count=1 ./...` once per rooted `go.mod` containing a `*_test.go` baseline. JavaScript runs Node’s test runner with TAP output and explicit discovered `.test`/`.spec` JavaScript paths, chunked below the sandbox argument limit. Python runs `python3 -m unittest discover -v` only when `test_*.py` files show stdlib unittest use. Missing tests and pytest-only layouts return `ErrUnsupported`.

Every preset uses 20 changed files, 64 KiB patch, 8 turns, 900 seconds, and one minimum test. Input snapshots allow 4096 guest-safe paths and 32 MiB aggregate content. Commands use operator-provided toolchains and offline dependency images; recipe construction never claims installation or execution.

Dependency manifests, tests, lockfiles, test configuration, scripts, and workflow paths are sorted and protected for the final patch gate. Unsupported ecosystems and unsafe paths fail closed.
