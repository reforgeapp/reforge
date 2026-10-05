package kubernetes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testHTTPClient(t *testing.T, server *httptest.Server) (*HTTPPodClient, string) {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := netSplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	certificate := server.TLS.Certificates[0].Certificate[0]
	caPath := t.TempDir() + "/ca.crt"
	if err = os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate}), 0600); err != nil {
		t.Fatal(err)
	}
	tokenPath := t.TempDir() + "/token"
	if err = os.WriteFile(tokenPath, []byte("token-one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := newHTTPPodClient(APIConfig{Namespace: "reforge", TokenFile: tokenPath, CAFile: caPath}, host, port)
	if err != nil {
		t.Fatal(err)
	}
	return client, tokenPath
}

func netSplitHostPort(value string) (string, string, error) {
	parsed, err := url.Parse("https://" + value)
	if err != nil {
		return "", "", err
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	return host, port, err
}

func validTestSpec() PodSpec {
	return PodSpec{
		Ref:    PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567"},
		Labels: map[string]string{"reforge.io/workspace": "rf-ws-0123456789abcdef01234567"},
		Image:  "ghcr.io/reforge/workspace-go@sha256:" + strings.Repeat("a", 64), RuntimeClassName: "gvisor",
		ContainerName: "workspace", WorkingDirectory: "/workspace", Environment: []string{"HOME=/tmp"}, ImagePullSecrets: []string{"registry-creds", "shared.pull-auth"},
		RunAsUser: 65532, RunAsNonRoot: true, RunAsGroup: 65532, FSGroup: 65532, ReadOnlyRootFilesystem: true,
		DropCapabilities: []string{"ALL"}, SeccompProfile: "RuntimeDefault", WorkspaceEmptyDirBytes: 1 << 30, TempEmptyDirBytes: 1 << 30,
		Resources: Resources{MemoryBytes: 512 << 20, CPUs: 2, DiskBytes: 1 << 30, PidsLimit: 128}, ActiveDeadlineSeconds: 60, RestartPolicy: "Never", NetworkProfile: "none",
	}
}

func TestHTTPPodClientCRUDAuthRotationAndRestrictedPod(t *testing.T) {
	var client *HTTPPodClient
	var tokenPath string
	var createBody map[string]any
	var authValues []string
	deleteCalls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authValues = append(authValues, r.Header.Get("Authorization"))
		if r.URL.Path == "/api/v1/namespaces/other/pods" {
			t.Errorf("request escaped configured namespace: %s", r.URL.Path)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/namespaces/reforge/pods":
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Error(err)
			}
			writeTestPod(w, "Pending")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/reforge/pods/rf-ws-0123456789abcdef01234567":
			writeTestPod(w, "Running")
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/namespaces/reforge/pods/rf-ws-0123456789abcdef01234567":
			deleteCalls++
			if deleteCalls > 1 {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			preconditions := body["preconditions"].(map[string]any)
			if preconditions["uid"] != "pod-uid" {
				t.Errorf("UID precondition missing: %#v", body)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, tokenPath = testHTTPClient(t, server)
	pod, err := client.Create(context.Background(), validTestSpec())
	if err != nil || pod.Ref.UID != "pod-uid" || pod.Phase != "Pending" {
		t.Fatalf("create pod=%+v err=%v", pod, err)
	}
	pod, err = client.Get(context.Background(), pod.Ref)
	if err != nil || pod.Phase != "Running" {
		t.Fatalf("get pod=%+v err=%v", pod, err)
	}
	if err = os.WriteFile(tokenPath, []byte("token-two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = client.Delete(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}); err != nil {
		t.Fatal(err)
	}
	if err = client.Delete(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}); err != nil {
		t.Fatalf("delete absent pod: %v", err)
	}
	if len(authValues) != 4 || authValues[0] != "Bearer token-one" || authValues[1] != "Bearer token-one" || authValues[2] != "Bearer token-two" || authValues[3] != "Bearer token-two" {
		t.Fatalf("service account token not reread: %v", authValues)
	}
	spec := createBody["spec"].(map[string]any)
	container := spec["containers"].([]any)[0].(map[string]any)
	security := container["securityContext"].(map[string]any)
	if spec["runtimeClassName"] != "gvisor" || spec["automountServiceAccountToken"] != false || spec["hostNetwork"] != nil || spec["hostPID"] != nil || spec["hostIPC"] != nil || security["runAsUser"] != float64(65532) || security["readOnlyRootFilesystem"] != true {
		t.Fatalf("pod security fields wrong: %#v", createBody)
	}
	imagePullSecrets := spec["imagePullSecrets"].([]any)
	if len(imagePullSecrets) != 2 || imagePullSecrets[0].(map[string]any)["name"] != "registry-creds" || imagePullSecrets[1].(map[string]any)["name"] != "shared.pull-auth" {
		t.Fatalf("image pull secret references missing or changed: %#v", spec["imagePullSecrets"])
	}
	volumes := spec["volumes"].([]any)
	if len(volumes) != 2 || strings.Contains(string(mustJSON(createBody)), "hostPath") || strings.Contains(string(mustJSON(createBody)), "pids") {
		t.Fatalf("unsafe or unsupported pod resources emitted: %#v", createBody)
	}
}

func writeTestPod(w http.ResponseWriter, phase string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"metadata":{"namespace":"reforge","name":"rf-ws-0123456789abcdef01234567","uid":"pod-uid","labels":{"reforge.io/workspace":"rf-ws-0123456789abcdef01234567"}},"status":{"phase":"`+phase+`"}}`)
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}

func TestHTTPPodClientExecWebSocketStreamsAndStatus(t *testing.T) {
	var gotAuth string
	var gotQuery url.Values
	upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, frame, err := conn.ReadMessage()
			if err != nil || len(frame) >= 2 && frame[0] == 255 && frame[1] == 0 {
				break
			}
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{1, 'o', 'u', 't'})
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{2, 'e', 'r', 'r'})
		status, _ := json.Marshal(map[string]any{"status": "Failure", "reason": "NonZeroExitCode", "details": map[string]any{"causes": []any{map[string]string{"reason": "ExitCode", "message": "7"}}}})
		_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte{3}, status...))
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte{255, 1})
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer server.Close()
	client, _ := testHTTPClient(t, server)
	var stdout, stderr bytes.Buffer
	code, err := client.Exec(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}, []string{"/opt/reforge/tool", "apply"}, strings.NewReader("input"), &stdout, &stderr)
	if err != nil || code != 7 || stdout.String() != "out" || stderr.String() != "err" {
		t.Fatalf("exec code=%d stdout=%q stderr=%q err=%v", code, stdout.String(), stderr.String(), err)
	}
	if gotAuth != "Bearer token-one" || gotQuery.Get("container") != "workspace" || gotQuery.Get("tty") != "false" || gotQuery["command"][0] != "/opt/reforge/tool" || gotQuery["command"][1] != "apply" {
		t.Fatalf("exec request auth=%q query=%v", gotAuth, gotQuery)
	}
}

func TestHTTPPodClientExecRejectsMalformedStatusAndOutputOverflow(t *testing.T) {
	for _, test := range []struct {
		name  string
		frame []byte
		want  error
	}{
		{name: "malformed", frame: []byte{3, '{'}, want: ErrBoundary},
		{name: "oversized", frame: append([]byte{1}, bytes.Repeat([]byte{'x'}, (6<<20)+1)...), want: ErrOutputLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				_, _, _ = conn.ReadMessage()
				_ = conn.WriteMessage(websocket.BinaryMessage, test.frame)
			}))
			defer server.Close()
			client, _ := testHTTPClient(t, server)
			_, err := client.Exec(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}, []string{"true"}, nil, io.Discard, io.Discard)
			if !errors.Is(err, test.want) {
				t.Fatalf("exec error=%v, want %v", err, test.want)
			}
		})
	}
}

func TestHTTPPodClientExecCancellationAndUIDFence(t *testing.T) {
	started := make(chan struct{})
	upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		close(started)
		_, _, _ = conn.ReadMessage()
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	client, _ := testHTTPClient(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Exec(ctx, PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}, []string{"sleep", "60"}, nil, io.Discard, io.Discard)
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("exec did not stop after context cancellation")
	}
	if err := client.Delete(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567"}); !errors.Is(err, ErrBoundary) {
		t.Fatalf("delete without UID accepted: %v", err)
	}
}

func TestHTTPPodClientRejectsArbitraryEndpointAndMalformedExecStatus(t *testing.T) {
	if _, err := newHTTPPodClient(APIConfig{Namespace: "reforge", TokenFile: "token", CAFile: "ca"}, "example.com", strconv.Itoa(443)); !errors.Is(err, ErrBoundary) {
		t.Fatalf("arbitrary host accepted: %v", err)
	}
	if _, err := parseExecStatus([]byte(`{"status":"Mystery"}`)); !errors.Is(err, ErrBoundary) {
		t.Fatalf("unknown status accepted")
	}
}

func TestHTTPPodClientExecStreamsLargePatchAndArtifactResponse(t *testing.T) {
	var received int
	artifactContent := bytes.Repeat([]byte{'a'}, 4<<20)
	artifactJSON, _ := json.Marshal(map[string][]byte{"content": artifactContent})
	upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if len(frame) > 1 && frame[0] == 0 {
				received += len(frame) - 1
			}
			if len(frame) == 2 && frame[0] == 255 && frame[1] == 0 {
				break
			}
		}
		_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte{1}, artifactJSON...))
		status := []byte(`{"status":"Success"}`)
		_ = conn.WriteMessage(websocket.BinaryMessage, append([]byte{3}, status...))
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer server.Close()
	client, _ := testHTTPClient(t, server)
	patch := bytes.Repeat([]byte{'x'}, 2<<20)
	var output bytes.Buffer
	_, err := client.Exec(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}, []string{"/opt/reforge/tool"}, bytes.NewReader(patch), &output, io.Discard)
	if err != nil || received != len(patch) || output.Len() != len(artifactJSON) {
		t.Fatalf("exec err=%v patch bytes=%d/%d artifact output=%d/%d", err, received, len(patch), output.Len(), len(artifactJSON))
	}
	var response struct {
		Content []byte `json:"content"`
	}
	if json.Unmarshal(output.Bytes(), &response) != nil || len(response.Content) != len(artifactContent) || base64.StdEncoding.EncodeToString(response.Content) != base64.StdEncoding.EncodeToString(artifactContent) {
		t.Fatal("artifact response was not streamed intact")
	}
}

type failedReader struct {
	done bool
}

func (r *failedReader) Read(data []byte) (int, error) {
	if !r.done {
		r.done = true
		return copy(data, []byte("x")), nil
	}
	return 0, errors.New("input failed")
}

func TestHTTPPodClientExecInputFailureClosesWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, _ := testHTTPClient(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.Exec(ctx, PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}, []string{"/opt/reforge/tool"}, &failedReader{}, io.Discard, io.Discard)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("input failure did not immediately close websocket: %v", err)
	}
}

func TestHTTPPodClientDoesNotFollowAPIRedirects(t *testing.T) {
	var redirected bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			redirected = true
			writeTestPod(w, "Running")
			return
		}
		http.Redirect(w, r, "/target", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, _ := testHTTPClient(t, server)
	_, err := client.Get(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567"})
	if err == nil || redirected {
		t.Fatalf("redirect followed=%v err=%v", redirected, err)
	}
}

func TestHTTPPodClientRequiresV5WebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{Subprotocols: []string{"v4.channel.k8s.io"}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()
	client, _ := testHTTPClient(t, server)
	_, err := client.Exec(context.Background(), PodRef{Namespace: "reforge", Name: "rf-ws-0123456789abcdef01234567", UID: "pod-uid"}, []string{"true"}, nil, io.Discard, io.Discard)
	if !errors.Is(err, ErrBoundary) {
		t.Fatalf("unsupported WebSocket protocol accepted: %v", err)
	}
}

func TestHTTPPodClientEnsureClaimCreatesOnlyMissingClaims(t *testing.T) {
	existing := map[string]bool{}
	var created []map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/reforge/persistentvolumeclaims/")
		switch {
		case r.Method == http.MethodGet && existing[name]:
			_, _ = io.WriteString(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/namespaces/reforge/persistentvolumeclaims":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			created = append(created, body)
			existing[body["metadata"].(map[string]any)["name"].(string)] = true
			_, _ = io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, _ := testHTTPClient(t, server)
	claim := Cache{StorageClass: "px-pool-rwx", AccessMode: "ReadWriteMany", Bytes: 10 << 30}.claim("org-1")
	for range 2 {
		if err := client.EnsureClaim(context.Background(), claim); err != nil {
			t.Fatal(err)
		}
	}
	if len(created) != 1 {
		t.Fatalf("created %d claims", len(created))
	}
	spec := created[0]["spec"].(map[string]any)
	if spec["storageClassName"] != "px-pool-rwx" || spec["accessModes"].([]any)[0] != "ReadWriteMany" {
		t.Fatalf("claim spec %#v", spec)
	}
	if other := (Cache{Bytes: 1 << 30}).claim("org-2"); other.Name == claim.Name {
		t.Fatal("tenants share a cache claim")
	}
	body := makePodBody(PodSpec{CacheClaim: claim.Name})
	volumes := body["spec"].(map[string]any)["volumes"].([]map[string]any)
	if volumes[len(volumes)-1]["persistentVolumeClaim"].(map[string]string)["claimName"] != claim.Name {
		t.Fatalf("cache volume missing: %#v", volumes)
	}
	if body["spec"].(map[string]any)["securityContext"].(map[string]any)["fsGroupChangePolicy"] != "OnRootMismatch" {
		t.Fatal("cache ownership would be rewritten on every mount")
	}
}
