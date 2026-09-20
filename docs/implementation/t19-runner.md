# T19 runner preparation

Runner preparation is offline and operator-driven. It builds the approved Go, JavaScript, and Python images with the existing packer, asks the compiled runner CLI for each image digest, hashes the selected runsc and sandbox tool binaries, and writes one mode-0600 runtime configuration. The command refuses an existing output or state path, symlinked inputs, group/world-writable files or directories, and non-executable runtime binaries.

Production requires delegated cgroup isolation. Development mode must be explicit and omits cgroup configuration; it is suitable only for loopback disposable testing. The script never downloads packages, calls a registry, enrolls a runner, registers policy, enables fixture authentication, or prints secrets.

Prepare from a checkout with the runner binary built:

~~~sh
python3 scripts/prepare-runner.py \
  --output /private/reforge-runner \
  --runner-cli /private/reforge/bin/reforge-runner \
  --runsc /usr/local/bin/runsc \
  --tool /usr/local/bin/reforge-sandbox-tool \
  --state-root /private/reforge-runner-state \
  --cgroup-root /sys/fs/cgroup/reforge
~~~

For an explicit local development stack:

~~~sh
python3 scripts/prepare-runner.py \
  --output /tmp/reforge-runner \
  --runner-cli "$PWD/bin/reforge-runner" \
  --runsc /private/tools/runsc \
  --tool /private/tools/reforge-sandbox-tool \
  --state-root /tmp/reforge-runner-state \
  --development --rootless
~~~

The command prints the runtime configuration path and a JSON REFORGE_REPAIR_IMAGES mapping from recipe names to selected image digests. Set that mapping in the server environment after reviewing the recorded digests. The runtime JSON separately maps each digest to its local image root. Start the enrolled worker only after creating a private credential file:

~~~sh
bin/reforge-runner enroll --endpoint https://reforge.example --credentials /private/runner.json --token-file /private/enrollment.token --name repair-worker
bin/reforge-runner run --endpoint https://reforge.example --credentials /private/runner.json --runtime-config /private/reforge-runner/runtime-config.json
~~~

Add --development to run only for the explicit development configuration. Keep controller CA selection and credential files private. The preparation output is a reproducibility aid; it does not certify hosted isolation or permit unreviewed runtime paths.

The disposable preparation check built all three stacks with the existing packer, obtained all three digests through the runner CLI, verified the mode-0600 JSON and confirmed the run command parsed it before rejecting an intentionally invalid credential. Binary hashes record selected operator paths; they do not establish binary authenticity. No network or paid model call was used.

Before enrollment, validate the generated configuration through the runner's local runtime verifier. It loads the strict JSON, checks selected binary hashes, image digests, resource bounds, and the development or production cgroup requirement by constructing and closing the sandbox runtime:

~~~sh
bin/reforge-runner verify-runtime --runtime-config /private/reforge-runner/runtime-config.json
~~~

Use `--development` only with a development configuration. The verifier does not enroll, contact a controller, execute repository code, or prove that an operator-supplied runsc binary is authentic; verify that binary separately and use the approved runtime build.

The real local gVisor check used `/tmp/reforge-gui-1789911741776198591/runtime-config.json`; `verify-runtime --development` exited successfully with `/tmp/reforge-gvisor/bin/runsc` and the built `bin/reforge-sandbox-tool`. The Go recipe digest was `sha256:e3c05191a7d519185395855f95ef9e9a51f5359152bc59f644215a28ba962852`.
