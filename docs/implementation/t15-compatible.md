# T15 compatible inference adapter

The default profile is OpenAI Chat Completions at the configured fixed `/v1` route. `ollama` and `vllm` are explicit deployment profiles with the same protocol and are independently qualified through their returned model metadata and an enrolled private endpoint. The `responses` profile is an explicit opt in that delegates directly to the reviewed OpenAI Responses adapter; there is no protocol fallback.

All traffic uses the injected HTTP client and a fixed HTTP(S) endpoint. Redirects are rejected for an injected `http.Client`; credentials are sent only in the authorization header. The adapter does not read environment credentials, use proxies or default clients, cache credentials, or retry paid inference requests.

Model listing and metadata are read only. If `/models` is absent, `ListModels` returns an actionable capability unknown error and `Probe` returns unknown capabilities so a configured model can still be qualified through its approved private route. Returned context and output limits are enforced before dispatch using a conservative request byte estimate and an explicit positive output bound.

Chat requests are stateless native message arrays. Continuations contain the complete native history through the completed assistant turn; system instructions and tool declarations are supplied separately on each request. Streaming text is emitted as it arrives, while tool calls are buffered by index and emitted only after `[DONE]`, a nonempty finish reason, and shared `model.CompileTools` validation. Missing completion markers, malformed events, partial calls, cancellation, and transport failures after dispatch are uncertain.

Requests are capped at `model.MaxRequestBytes`; stream lines and total stream bytes are bounded. Usage is known only for a completed response with nonnegative prompt and completion counts. Cached prompt tokens are reported when the endpoint provides them.

The wire profile follows the official [OpenAI Chat Completions reference](https://platform.openai.com/docs/api-reference/chat), [Ollama OpenAI compatibility documentation](https://docs.ollama.com/api/openai-compatibility), and [vLLM OpenAI compatible server documentation](https://docs.vllm.ai/en/latest/serving/online_serving/openai_compatible_server/). Fixture tests cover text and parallel tools, usage and continuation, malformed and incomplete streams, empty tools, cancellation, unavailable model listing, model limits, and output cap enforcement. No paid endpoint was used. A real local endpoint was subsequently verified as described below.


## Local endpoint verification

Coordinator tested official Ollama 0.34.2 on loopback with cloud functionality disabled and Qwen3:0.6b. Its release archive matched SHA256 `e155b83589986d2c581fdb1381ea3ebdb16549883679cd5a0627f7cdc05b12b`. Both the Chat Completions and Responses profiles completed an actual schema-valid tool call followed by a second turn using native continuation and reported usage. The opt-in integration test passed in 11.192 seconds. Run `REFORGE_LOCAL_OLLAMA_TEST=1 go test -run TestRealLocalOllamaProfiles -v ./internal/model/compatible` with that local server/model available; the test uses an explicit loopback private-route binding.

This verifies these protocols on the tested Ollama/model combination. It does not certify vLLM, arbitrary compatible endpoints, hosted isolation or enrolled private model transport. T07/T19 integration must verify the latter and durable budget admission before inference. Model-list metadata alone never qualifies tool execution or a billing limit. Invalid/duplicate metadata, changing stream IDs, excessive tool indices, incomplete terminal reasons, oversized continuations and malformed usage fail closed or retain unknown capability/usage. Complete history and exact JSON numbers survive continuation.

Ollama tool turns request `reasoning_effort: none` and `temperature: 0`; output and wall-clock ceilings remain enforced. Local Qwen3 reasoning otherwise exhausted the bounded output stream before emitting a tool call. These fields are documented in [Ollama's compatibility API](https://docs.ollama.com/api/openai-compatibility). Other compatible profiles keep their existing provider defaults. Model quality and complete terminal usage still require qualification.
