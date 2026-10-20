package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/domain"
	"github.com/reforgeapp/reforge/pkg/maintenance/repair"
)

func TestRepairSourceMovedUsesTypedConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	repairFailure(ctx, repair.ErrSourceMoved)
	var body domain.Error
	if writer.Code != 409 || json.Unmarshal(writer.Body.Bytes(), &body) != nil || body.Code != "source_moved" || body.Retryable {
		t.Fatalf("status=%d body=%s", writer.Code, writer.Body.String())
	}
}
