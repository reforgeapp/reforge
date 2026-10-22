package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reforgeapp/reforge/pkg/config"
)

func TestRepairRequestBodyLimitExceedsOrdinaryLimit(t *testing.T) {
	server := New(config.Config{Development: true}, nil)
	readBody := func(c *gin.Context) {
		_, err := io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusNoContent)
	}
	server.Router.POST("/runner/v1/repair/checkpoint", readBody)
	server.Router.POST("/runner/v1/ordinary", readBody)
	body := strings.Repeat("x", 3<<20)

	request := httptest.NewRequest(http.MethodPost, "/runner/v1/repair/checkpoint", strings.NewReader(body))
	repairResponse := httptest.NewRecorder()
	server.Router.ServeHTTP(repairResponse, request)
	if repairResponse.Code != http.StatusNoContent {
		t.Fatalf("repair body >2MiB rejected: %d", repairResponse.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/runner/v1/ordinary", strings.NewReader(body))
	ordinaryResponse := httptest.NewRecorder()
	server.Router.ServeHTTP(ordinaryResponse, request)
	if ordinaryResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("ordinary body >2MiB accepted: %d", ordinaryResponse.Code)
	}
}
