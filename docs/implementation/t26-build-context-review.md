# T26 build-context secret review

`.dockerignore` excludes root and nested `.env` files, including suffixed variants, while retaining `.env.example` templates. It also excludes the default `deploy/compose/runner` bind-mount directory and any directory named `credentials`.

A disposable Docker context check used generated sentinel values in root `.env`/`.env.production.local`, `web/.env`/`web/.env.local`, and the default runner credentials/runtime config. Go's `COPY . .` context and web's `COPY web ./` context both passed absence checks. The Compose `.env.example` template remained present. Docker used its legacy builder because the configured buildx plugin was missing; both checks completed successfully. Temporary marker files, containers and tagged check images were removed.

This protects only conventional paths in Docker build contexts. A credential stored under another filename or an alternate `REFORGE_RUNNER_DIR` inside the repository can still enter `COPY . .`; keep arbitrary secrets outside the repository/build context and inspect custom build inputs.
