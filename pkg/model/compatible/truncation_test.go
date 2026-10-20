package compatible

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/model"
)

func TestOutputLimitSettlesReportedUsageWithoutExecutingPartialTools(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"limited","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"partial","type":"function","function":{"name":"lookup","arguments":"{\"key\":"}}]},"finish_reason":"length"}]}`,
		`data: {"id":"limited","choices":[],"usage":{"prompt_tokens":25,"completion_tokens":32,"total_tokens":57}}`,
		`data: [DONE]`,
	}, "\n\n")
	p := fixtureProvider(t, &fixtureClient{responses: []*http.Response{fixtureResponse(200, stream)}})
	result, err := model.CollectTurn(context.Background(), p, model.Turn{OperationID: domain.NewID(), Model: "local-model", Messages: []model.Message{{Role: "user", Text: "lookup"}}, Tools: []model.Tool{{Name: "lookup", Schema: []byte(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`)}}, MaxOutputTokens: 32, TimeoutMS: 1000})
	if err != nil || result.FinishReason != "length" || !result.Usage.Known || result.Usage.OutputTokens != 32 || len(result.ToolCalls) != 0 || len(result.Continuation) != 0 {
		t.Fatalf("truncated response accounting or tool isolation: finish=%s usage=%+v calls=%d continuation=%d error=%v", result.FinishReason, result.Usage, len(result.ToolCalls), len(result.Continuation), err)
	}
}
