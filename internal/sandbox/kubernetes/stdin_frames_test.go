package kubernetes

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestExecInputFramesFitAPIServerLimit(t *testing.T) {
	sizes := make(chan int, 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil || len(payload) > 0 && payload[0] == 255 {
				close(sizes)
				return
			}
			sizes <- len(payload)
		}
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = writeExecInput(context.Background(), conn, bytes.NewReader(bytes.Repeat([]byte("x"), 20000)), make(chan struct{})); err != nil {
		t.Fatal(err)
	}
	total := 0
	for size := range sizes {
		if size > 4096 {
			t.Fatalf("stdin frame of %d bytes exceeds the API server's 4096-byte bridge limit", size)
		}
		total += size - 1
	}
	if total != 20000 {
		t.Fatalf("sent %d bytes, want 20000", total)
	}
}
