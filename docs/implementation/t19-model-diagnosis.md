# T19 compatible model diagnosis

Live repair logs separate two failures.

The `qwen4-final` run stopped after 97.594 seconds with `code=protocol`, `uncertain=true`, HTTP 409, `turns=0`, and no candidate. This confirms adapter-level protocol rejection, but raw provider finish reason/schema was not logged. “Length” or unsupported Ollama framing is an inference, not an observed fact. At that run, the adapter accepted only completed `stop` or nonempty `tool_calls`. Current code also settles a terminal `length` response when the stream and usage are complete, discards partial tools, and stops with an actionable output-limit handoff. Incomplete or unknown usage remains uncertain. Zero returned tools does not prove provider emitted zero tools: the parser validates calls before emitting them.

The `protocol` run reached six turns and returned read, target-read, patch, and check calls. History continued through the engine correctly. Failure was model behavior: validation showed JavaScript expected `2` but received `{ value: 2 }`; later patches redeclared `value`. No tool-history corruption is evidenced.

Recommended fix: record bounded finish reason/profile/model diagnostics; qualify each Ollama/Qwen route with a tool-call plus continuation probe; keep failed routes disabled and never retry uncertain usage automatically. Do not loosen completion validation. Prompt/model tuning may reduce bad patches, but does not establish adapter correctness.

The later `qwen8` log demonstrates normal protocol/tool history: eight successful model-turn HTTP responses returned read/read-target, run_checks, and apply_patch calls. The candidate passed its upgraded-dependency check (`value: pass`, exit 0), but the run ended at the configured eight-turn ceiling before producing a patch that passed both upgraded and original target validation. The final model response contained no tools and proposed a compatibility hypothesis; this is model convergence/turn-budget failure, not parser failure. Turn durations total roughly 134 seconds, so increasing wall time alone is unlikely to help.

Next bounded step: retain the existing maximum and reserve one explicit compatibility-repair turn only when target validation fails, with a hard total turn cap and unchanged budget reservation. Record baseline and target outcomes per turn. Do not treat a passing candidate as success until target validation and patch identity both pass.

Raw fixture capture is now available with `REFORGE_CAPTURE_MODEL=1`; bodies are bounded to1MiB each under private `.local/model-capture/test-*` directories. Only synthetic local Ollama request JSON and response SSE are captured, without headers. Other probe requests pass through without capture.

The input-dependent Qwen1.7b run in `.local/repair-wire-diagnostic.log` failed61.02s. Seven responses had complete terminal markers and known usage; request history contained the correct tool replies and source/test content. The model misinterpreted an arithmetic assertion as equality and applied an incorrect boolean-returning patch. This case is model reasoning failure, not transport or continuation loss.

Qwen8b input-dependent upgrade failed106.67s at eight turns (`.local/repair-upgrade-cache-diagnostic.log`). Candidate passed; target did not. Recipe v2 permits12 turns while preserving hard budget/time checks. Read-only turns now reuse unchanged validation evidence; changed patches invalidate both candidate and target checks. New qualification run remains pending.

Recipe v2 exhausted12 turns after the correct diagnosis (`.local/repair-upgrade-v2.log`,145.083s). Versioned recipe v3 permits16 turns; real controller/runner/Ollama/Gitea dependency-upgrade acceptance passed153.769s (`.local/repair-upgrade-v3.log`). Both dependency revisions and native publication were verified. The Go browser fixture still fails at16 turns with no verified candidate (`.local/repair-browser-v3.log`); bounded wire capture is being added for diagnosis.
