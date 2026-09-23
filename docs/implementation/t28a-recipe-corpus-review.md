# Maintenance recipe corpus review

Status: bounded local corpus complete; V08/V22 remain partial. No live-provider or G5 claim.

## Recipe outcomes

Ran `TestToolchainImagesRunFrozenRecipes` with `REFORGE_RUN_TOOLCHAIN_INTEGRATION=1` on
2026-09-23. Recipe version was v3; each workspace used a content-addressed image and the
supplied `runsc` release-20260914.0. Each one-test fixture failed on baseline, accepted one
source-only patch, then passed the unchanged test on candidate:

| Recipe | Baseline | Candidate | Test duration |
| --- | --- | --- | ---: |
| Go | `TestValue` failed, exit 1 | passed, exit 0 | 12.90 s |
| JavaScript | `value` failed, exit 1 | passed, exit 0 | 1.99 s |
| Python unittest | `ValueTest.test_value` failed, exit 1 | passed, exit 0 | 3.63 s |

Full log: `.local/t28a-recipe-corpus/toolchains.log` (ignored local artifact).

## Real model attempts

Ran `TestRepairEngineRealModelAndGVisor` against the local Ollama-compatible endpoint and
the frozen JavaScript recipe. Existing test has 1,024 output-token and 15-minute limits.
All four runs reached real inference; none produced a validated candidate:

| Profile | Elapsed | Outcome |
| --- | ---: | --- |
| `qwen3:0.6b` | 17.685 s | Handoff. Candidate used CommonJS in an ES module fixture; candidate test still failed with `Named export 'value' not found`. |
| `qwen3:1.7b` | 22.545 s | Handoff. Model attempted to rewrite protected `value.test.js`; patch was refused. Subsequent tool arguments failed the registered schema; no candidate validated. |
| `qwen3:4b` | 94.621 s | Handoff before patch. Model reached configured output-token limit. |
| `qwen2.5:3b` | 74.263 s | Handoff after bounded turns. Three source candidates failed unchanged test; last introduced diagnostic `console.log`, none passed. |

Per-profile logs: `.local/t28a-recipe-corpus/qwen*.log` (ignored local artifacts). The
1.7B run is direct evidence of one denied model test-rewrite attempt; no successful bypass or
candidate validation followed. These models are local development profiles, not provider
certification or an accuracy benchmark.

## Tampering guards

Uncached `go test -count=1 ./internal/maintenance/repair ./internal/maintenance/recipes`
passed. `TestPatchCannotChangeValidationOrSensitivePaths` rejects test, manifest, script,
private, delete, bypass-code, `.git`, and oversized patches; it also rejects changed protected
hashes and altered frozen plan commands. `TestEngineStopsRepeatedRejectedTestRewrites`
confirms repeated test edits stop before candidate validation. `TestValidateCustomRequiresBaselineAndTarget`
rejects protected edits and missing baseline/target evidence. No guard suite failures or
accepted tampering observed.

## Remaining qualification

Corpus is three minimal single-test smoke fixtures, not representative maintenance accuracy
or a hostile-repository corpus. Real model evaluation covered JavaScript only and yielded
zero successful repairs across four local profiles. Repeat on representative Go/JS/Python
repositories with qualified model profiles, larger bounded sample, adversarial prompt corpus,
and collect cost/quality distributions before claiming V22. V08's local guard evidence is
present, while end-to-end real-provider repair remains unverified.
