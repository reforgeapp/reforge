# T14 Google native model adapter

The adapter uses `google.golang.org/genai` v1.71.0 with the Gemini Developer API backend and an injected fixed-origin HTTP client. API keys are explicit; environment credentials, OAuth, Gemini CLI identity and paid capability probes are not used. SDK retries are limited to one attempt. Vertex AI remains unsupported until the shared connection contract includes explicit project, location and credentials.

`Probe` and `ListModels` use the read-only models endpoint. Model metadata establishes only advertised capabilities; streaming, tool calling, signatures and usage remain unknown until qualified. Generation is pinned to the configured model and requires a positive output-token bound.

`StreamTurn` retains the SDK transport, authentication, streaming parser and text events. Its request-local bounded capture preserves raw native response content for tools and continuations. The pinned SDK converts generic JSON numbers through `float64`; its documented `HTTPOptions.ExtrasRequestProvider` restores raw contents and tool schemas at final request serialization. Tool arguments and responses retain integer precision through schema validation and replay. Concurrent turns have separate capture buffers.

`Event.Continuation` is the complete native content array through the finished turn. Part order, signature attachment, thought parts and unknown native content fields are retained. Continuation plus new messages supplies the next turn. Missing native function IDs receive a deterministic internal identity based on content, history position and part position. Native history and function responses retain the original absence of an ID. Existing native IDs pass through unchanged; duplicate or ambiguous IDs are rejected.

Tools are emitted only after complete stream EOF, successful schema validation, a `STOP` finish and bounded continuation serialization. Other finish reasons cannot release buffered calls. The pinned GenerateContent Gemini profile rejects `partialArgs` and `willContinue`, which its SDK defines as unsupported on this backend. Invalid candidates, identity changes, trailing candidate content and incomplete streams fail with uncertain usage.

Usage is known only when native prompt and candidate count fields are present, nonnegative and internally consistent. Empty metadata is unknown. Candidate and thought counts are summed as 64-bit integers; cache counts remain a prompt subset. Missing counts, inconsistent totals and unqualified tool-use prompt accounting remain unknown. Estimates never authorize spending.

Requests and continuations are capped at 1 MiB; response bodies, including streaming tails after a terminal frame, are capped at 8 MiB. Overflow is an error rather than EOF. Bodies close on success, cancellation and failure. Callback failures after dispatch are uncertain and do not expose callback text.

Sources: official [GenerateContent API](https://ai.google.dev/api/generate-content), [function calling guide](https://ai.google.dev/gemini-api/docs/function-calling), [thinking guide](https://ai.google.dev/gemini-api/docs/thinking), and the pinned SDK's `api_client.go`, `types.go` and `common.go`. This profile implements GenerateContent, not the separate Interactions API.

Local fixture tests cover ordered signed parallel calls, missing native IDs, complete continuation replay, integers above 2^53, duplicate IDs, blocked finishes, unsupported partial arguments, missing usage, overflow, null content, callback failure, oversized terminal tails, body closure, explicit key selection, cancellation and concurrent capture isolation. No live paid API calls were made.

```sh
GOPATH=/tmp/reforge-go GOMODCACHE=/tmp/reforge-go-mod GOCACHE=/tmp/reforge-go-build go test -race ./internal/model/google
```
