# Local development database credential rotation

Rotated the disposable PostgreSQL roles `mnorris`, `reforge_migrator`, and `reforge_runtime` with fresh random passwords. Updated `.local/development.env` and `.local/pg-admin-password` with atomic per-file replacement while the local Reforge server was stopped; the database role changes were one transaction. Both files remain mode 0600. Development and test database URLs now use the new migrator/runtime credentials. The application encryption key was unchanged.

The local demo restarted on `127.0.0.1:8080`. PostgreSQL authenticated each admin, migrator, and runtime URL against the intended database (`reforge_dev` or `reforge_test`). The demo metadata endpoint and an authenticated connections read both returned HTTP 200; the read returned 20 records. Credential values were not printed or recorded here.
