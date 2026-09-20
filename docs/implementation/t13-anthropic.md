# T13 Anthropic Messages adapter

The adapter uses the official `github.com/anthropics/anthropic-sdk-go` client with the configured fixed-origin HTTP client, API key, and model. It disables SDK retries, does not resolve environment credentials, and rejects model changes or non-positive output limits. It sends only the API key authentication path; Claude subscription credentials are not discovered or accepted by this adapter.

`Probe` uses the read-only Models API and reports model metadata and capability states without a paid completion. `ListModels` walks a bounded complete pagination sequence and rejects repeated identities. Streaming uses Messages SSE and the SDK accumulator: text deltas become text events, indexed tool blocks collect partial JSON by content-block index, and tool calls are validated at `content_block_stop` and buffered until the terminal event. Incomplete blocks, malformed JSON, unknown tools, and schema failures stop the turn before a tool event.

Completed message usage is emitted only after `message_stop`, with present input/final-output fields and nonnegative bounded values. Input totals include cache reads and cache creation; cache creation is separate for premium billing. interrupted streams return an uncertain typed error and never report authoritative usage. Complete input and native assistant history is exposed as opaque continuation, preserving thinking signatures, redacted thinking, tool blocks, and their order. A following turn prepends that continuation before its new messages and tool results. Thinking content is never emitted as transcript text.

Errors are normalized without invoking SDK error formatting, which can include provider response bodies. Authentication, model, rate-limit, invalid-request, cancellation, transport, and protocol paths are bounded and redacted. Byte estimates remain unknown planning data; configured context/output limits determine reservations, including opaque reasoning. Pricing remains outside this package.

Fixture tests cover indexed parallel tools, partial JSON, malformed input, interrupted streams, usage, opaque continuation ordering, probe/model errors, cancellation, and estimates. No paid API key or live request was used, so live model capability, quota, and provider-version qualification remain deployment checks.

The implementation follows the official [API overview](https://platform.claude.com/docs/en/api/overview), [Create a Message reference](https://platform.claude.com/docs/en/api/messages/create), [streaming documentation](https://platform.claude.com/docs/en/build-with-claude/streaming), and [Go SDK guide](https://platform.claude.com/docs/en/cli-sdks-libraries/sdks/go).

Root review fixed full-history replay, empty-argument tools, duplicate tool-result text, metadata presence, stream/body closure, cache accounting and terminal buffering. Race/vet passed; the SDK round-trip fixture preserves signed thinking through two turns and verifies both HTTP bodies close.
