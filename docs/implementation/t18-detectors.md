# T18 dependency detectors

`internal/maintenance/detectors` compares bounded base and candidate manifest snapshots. It reports only added, removed, or changed dependency declarations, sorted by ecosystem, manifest, and name. `From` and `To` retain resolver supplied raw ranges; detector never selects a target version.

Supported files: `go.mod`, `package.json` dependency sets, PEP 621 and Poetry sections in `pyproject.toml`, and `requirements*.txt`. Unsupported files are ignored. Malformed, duplicate, or ambiguous supported declarations make `Complete` false and add actionable reasons. Missing manifests produce removals/additions. File, byte, and dependency limits prevent unbounded parsing.

`BotConfiguration` reports Renovate and Dependabot config presence and automerge as `enabled`, `disabled`, or `unknown`. Renovate JSON5, dynamic `extends`, package rules, malformed JSON, and multiple configs are unknown. Dependabot automerge is always unknown because native configuration has no authoritative automerge setting. It does not make actor, merge, or trust decisions.

Known limits: package manifests do not preserve dependency-set labels in the frozen result API; TOML and JSON parser diagnostics are intentionally concise; requirements options and editable/VCS forms are treated as ambiguous. Inputs are bounded by file count, aggregate bytes, and dependency count. Root fingerprints and cross-manifest deduplication belong to the maintenance coordinator.
