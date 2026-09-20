# T19 repair engine acceptance

`TestRepairEngineRealModelAndGVisor` is an explicit opt-in acceptance test for the real Qwen3 1.7b model at `127.0.0.1:55435` and the pinned local gVisor runtime. It builds a fresh JavaScript toolchain image, freezes a broken arithmetic fixture with an unchanged protected test, and runs `repair.Engine` through real baseline reproduction, model file reading, source patching, frozen checks, upgrade validation, and target validation. The model route uses the compatible Ollama adapter and `model.CollectTurn`; no model mock is used. `REFORGE_REPAIR_TEST_MODEL` may select another locally installed model for qualification.

The test also calls `ValidateNative` against a pinned in-memory target snapshot. This checks engine native validation isolation and is not publication or repository-authority certification. Artifact callbacks retain returned command bytes and require baseline, candidate, and target evidence.

Run:

```text
REFORGE_REPAIR_ENGINE_TEST=1 go test ./test/integration -run TestRepairEngineRealModelAndGVisor -count=1 -v
```

The test requires `/tmp/reforge-gvisor/bin/runsc`, `bin/reforge-sandbox-tool`, and the local Qwen fixture. Without explicit opt-in it skips; model or runtime failures remain test failures and report bounded tool-call summaries directly. The local `/no_think` directive is model steering for this fixture, not a repair instruction.

The earlier narrowly passing engine run used qwen3:1.7b with the Ollama adapter's `reasoning_effort:none` and temperature zero; it passed the baseline, candidate, target, and native C validation flow in 15.02 seconds. The result is recorded in `.local/engine-current.log`. An earlier qwen3:4b run (94.83 seconds) ended without a completed stream; raw diagnostics showed reasoning/content deltas without a terminal marker, so that model is not treated as qualified. The strengthened controller dependency-upgrade fixture subsequently passed153.769s with qwen3:8b and recipev3 (`.local/repair-upgrade-v3.log`); browser acceptance remains open; the15.02s engine result is not broad model-route certification.
