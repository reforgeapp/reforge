package integration

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const modelCaptureLimit int64 = 1 << 20

type modelCaptureState struct {
	seq int
	mu  sync.Mutex
}

type modelCaptureReadCloser struct {
	mu     sync.Mutex
	src    io.ReadCloser
	path   string
	data   []byte
	closed bool
}

func (r *modelCaptureReadCloser) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) < int(modelCaptureLimit) {
		remaining := int(modelCaptureLimit) - len(r.data)
		copyN := n
		if copyN > remaining {
			copyN = remaining
		}
		r.data = append(r.data, p[:copyN]...)
	}
	return n, err
}

func (r *modelCaptureReadCloser) Close() error {
	err := r.src.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.closed = true
		if writeErr := os.WriteFile(r.path, r.data, 0600); err == nil {
			err = writeErr
		}
	}
	return err
}

type modelCaptureWriter struct {
	http.ResponseWriter
	path    string
	data    []byte
	capture bool
}

func (w *modelCaptureWriter) WriteHeader(status int) {
	w.capture = isSSE(w.Header().Get("Content-Type"))
	w.ResponseWriter.WriteHeader(status)
}

func (w *modelCaptureWriter) Write(p []byte) (int, error) {
	if !w.capture {
		w.capture = isSSE(w.Header().Get("Content-Type"))
	}
	n, err := w.ResponseWriter.Write(p)
	if w.capture && len(w.data) < int(modelCaptureLimit) {
		remaining := int(modelCaptureLimit) - len(w.data)
		copyN := n
		if copyN > remaining {
			copyN = remaining
		}
		w.data = append(w.data, p[:copyN]...)
	}
	return n, err
}

func (w *modelCaptureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *modelCaptureWriter) finish() {
	if w.capture {
		_ = os.WriteFile(w.path, w.data, 0600)
	}
}

func captureLocalModel(t *testing.T, root string) string {
	t.Helper()
	if os.Getenv("REFORGE_CAPTURE_MODEL") != "1" {
		return "http://127.0.0.1:55435"
	}
	parent := filepath.Join(root, ".local", "model-capture")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(parent, "test-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	state := &modelCaptureState{}
	target, err := url.Parse("http://127.0.0.1:55435")
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			proxy.ServeHTTP(w, r)
			return
		}
		state.mu.Lock()
		state.seq++
		seq := state.seq
		state.mu.Unlock()
		capture := &modelCaptureWriter{ResponseWriter: w, path: filepath.Join(dir, "response-"+formatCaptureSeq(seq)+".sse")}
		defer capture.finish()
		if isJSON(r.Header.Get("Content-Type")) {
			r.Body = &modelCaptureReadCloser{src: r.Body, path: filepath.Join(dir, "request-"+formatCaptureSeq(seq)+".json")}
		}
		proxy.ServeHTTP(capture, r)
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Logf("synthetic local model wire capture: %s", dir)
	return server.URL
}

func isJSON(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && (mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"))
}

func isSSE(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "text/event-stream"
}

func formatCaptureSeq(seq int) string {
	return fmt.Sprintf("%06d", seq)
}
