package kubernetes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var ErrTransport = errors.New("Kubernetes API transport failed")
var ErrOutputLimit = errors.New("Kubernetes exec output limit exceeded")

type APIConfig struct {
	Namespace string
	TokenFile string
	CAFile    string
}

type HTTPPodClient struct {
	baseURL   url.URL
	namespace string
	tokenFile string
	http      *http.Client
	dialer    websocket.Dialer
}

func NewHTTPPodClient(cfg APIConfig) (*HTTPPodClient, error) {
	if cfg.Namespace == "" || !namespaceValue.MatchString(cfg.Namespace) {
		return nil, ErrBoundary
	}
	serviceHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	servicePort := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if servicePort == "" {
		servicePort = os.Getenv("KUBERNETES_SERVICE_PORT")
	}
	if cfg.TokenFile == "" {
		cfg.TokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	}
	if cfg.CAFile == "" {
		cfg.CAFile = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	}
	return newHTTPPodClient(cfg, serviceHost, servicePort)
}

func newHTTPPodClient(cfg APIConfig, serviceHost, servicePort string) (*HTTPPodClient, error) {
	if net.ParseIP(serviceHost) == nil || !namespaceValue.MatchString(cfg.Namespace) {
		return nil, ErrBoundary
	}
	port, err := strconv.Atoi(servicePort)
	if err != nil || port < 1 || port > 65535 || cfg.TokenFile == "" || cfg.CAFile == "" {
		return nil, ErrBoundary
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, errors.Join(ErrTransport, err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, ErrBoundary
	}
	base := url.URL{Scheme: "https", Host: net.JoinHostPort(serviceHost, servicePort), Path: "/api/v1"}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	transport := &http.Transport{TLSClientConfig: tlsConfig, Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ResponseHeaderTimeout: 15 * time.Second}
	return &HTTPPodClient{
		baseURL: base, namespace: cfg.Namespace, tokenFile: cfg.TokenFile,
		http:   &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		dialer: websocket.Dialer{TLSClientConfig: tlsConfig.Clone(), Proxy: nil, HandshakeTimeout: 15 * time.Second, Subprotocols: []string{"v5.channel.k8s.io"}},
	}, nil
}

func (c *HTTPPodClient) Create(ctx context.Context, spec PodSpec) (Pod, error) {
	if err := validPodSpec(spec); err != nil || spec.Ref.Namespace != c.namespace {
		return Pod{}, ErrBoundary
	}
	body, err := json.Marshal(makePodBody(spec))
	if err != nil || len(body) > 1<<20 {
		return Pod{}, ErrBoundary
	}
	var pod Pod
	response, err := c.request(ctx, http.MethodPost, c.resourceURL("pods"), body)
	if err == nil {
		pod, err = parsePod(response)
	}
	return pod, err
}

func (c *HTTPPodClient) EnsureClaim(ctx context.Context, claim Claim) error {
	if !strings.HasPrefix(claim.Name, "rf-cache-") || len(claim.Name) > 63 || claim.Bytes <= 0 {
		return ErrBoundary
	}
	if _, err := c.request(ctx, http.MethodGet, c.resourceURL("persistentvolumeclaims", claim.Name), nil); err == nil || !apiStatus(err, http.StatusNotFound) {
		return err
	}
	spec := map[string]any{"accessModes": []string{claim.AccessMode}, "resources": map[string]any{"requests": map[string]string{"storage": strconv.FormatInt(claim.Bytes, 10)}}}
	if claim.StorageClass != "" {
		spec["storageClassName"] = claim.StorageClass
	}
	body, err := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": claim.Name, "labels": map[string]string{"app.kubernetes.io/name": "reforge-cache"}}, "spec": spec})
	if err != nil {
		return ErrBoundary
	}
	if _, err = c.request(ctx, http.MethodPost, c.resourceURL("persistentvolumeclaims"), body); err != nil && !apiStatus(err, http.StatusConflict) {
		return err
	}
	return nil
}

func apiStatus(err error, code int) bool {
	return err != nil && strings.Contains(err.Error(), fmt.Sprintf("API status %d:", code))
}

func (c *HTTPPodClient) Get(ctx context.Context, ref PodRef) (Pod, error) {
	if err := c.validateRef(ref, false); err != nil {
		return Pod{}, err
	}
	var pod Pod
	response, err := c.request(ctx, http.MethodGet, c.resourceURL("pods", ref.Name), nil)
	if err == nil {
		pod, err = parsePod(response)
	}
	return pod, err
}

func (c *HTTPPodClient) List(ctx context.Context, labels map[string]string) ([]Pod, error) {
	if len(labels) == 0 || len(labels) > 2 {
		return nil, ErrBoundary
	}
	keys := make([]string, 0, len(labels))
	for key, value := range labels {
		if (key != "app.kubernetes.io/name" && key != "reforge.io/runner-id") || !labelValue.MatchString(value) {
			return nil, ErrBoundary
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	base, _ := url.Parse(c.resourceURL("pods"))
	var pods []Pod
	continuation := ""
	for page := 0; page < 50; page++ {
		target := *base
		query := target.Query()
		query.Set("labelSelector", strings.Join(parts, ","))
		query.Set("limit", "200")
		if continuation != "" {
			query.Set("continue", continuation)
		}
		target.RawQuery = query.Encode()
		data, err := c.request(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, err
		}
		var list struct {
			Metadata struct {
				Continue string `json:"continue"`
			} `json:"metadata"`
			Items []json.RawMessage `json:"items"`
		}
		if err = json.Unmarshal(data, &list); err != nil || len(pods)+len(list.Items) > 10000 {
			return nil, errors.Join(ErrTransport, err, ErrBoundary)
		}
		for _, item := range list.Items {
			pod, parseErr := parsePod(item)
			if parseErr != nil {
				return nil, parseErr
			}
			pods = append(pods, pod)
		}
		continuation = list.Metadata.Continue
		if continuation == "" {
			return pods, nil
		}
	}
	return nil, ErrBoundary
}

func (c *HTTPPodClient) Delete(ctx context.Context, ref PodRef) error {
	if err := c.validateRef(ref, true); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]string{"uid": ref.UID}, "gracePeriodSeconds": 0})
	return c.requestJSON(ctx, http.MethodDelete, c.resourceURL("pods", ref.Name), body, nil)
}

func (c *HTTPPodClient) Exec(ctx context.Context, ref PodRef, command []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if err := c.validateRef(ref, true); err != nil || len(command) == 0 || len(command) > 128 {
		return 0, ErrBoundary
	}
	for _, arg := range command {
		if len(arg) > 16384 || strings.ContainsRune(arg, 0) {
			return 0, ErrBoundary
		}
	}
	parsed, _ := url.Parse(c.resourceURL("pods", ref.Name, "exec"))
	query := parsed.Query()
	query.Set("container", "workspace")
	query.Set("stdin", "true")
	query.Set("stdout", "true")
	query.Set("stderr", "true")
	query.Set("tty", "false")
	for _, arg := range command {
		query.Add("command", arg)
	}
	parsed.RawQuery = query.Encode()
	parsed.Scheme = "wss"
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	token, err := c.readToken()
	if err != nil {
		return 0, err
	}
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, response, err := c.dialer.DialContext(ctx, parsed.String(), header)
	if err != nil {
		if response != nil {
			defer response.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
			err = fmt.Errorf("%w: %s %s", err, response.Status, strings.Join(strings.Fields(string(body)), " "))
		}
		return 0, errors.Join(ErrTransport, err)
	}
	defer conn.Close()
	if conn.Subprotocol() != "v5.channel.k8s.io" {
		return 0, ErrBoundary
	}
	conn.SetReadLimit((6 << 20) + (64 << 10))
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "context ended"), time.Now().Add(time.Second))
			_ = conn.Close()
		case <-watchDone:
		}
	}()
	stop := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		err := writeExecInput(ctx, conn, stdin, stop)
		writerDone <- err
		if err != nil {
			_ = conn.Close()
		}
	}()
	var output execOutput
	output.stdout = stdout
	output.stderr = stderr
	output.limit = 6 << 20
	var exitCode int
	gotStatus := false
	writerCompleted := false
	var writerErr error
	for {
		if err := ctx.Err(); err != nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "context ended"), time.Now().Add(time.Second))
			close(stop)
			return 0, err
		}
		messageType, payload, readErr := conn.ReadMessage()
		if readErr != nil {
			select {
			case inputErr := <-writerDone:
				writerCompleted = true
				writerErr = inputErr
				if inputErr != nil {
					close(stop)
					return 0, errors.Join(ErrTransport, inputErr)
				}
			default:
			}
			if gotStatus && websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				break
			}
			close(stop)
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			return 0, errors.Join(ErrTransport, readErr)
		}
		if messageType != websocket.BinaryMessage || len(payload) == 0 {
			close(stop)
			return 0, ErrBoundary
		}
		switch payload[0] {
		case 1, 2:
			if err := output.Write(payload[0], payload[1:]); err != nil {
				close(stop)
				return 0, err
			}
		case 3:
			code, err := parseExecStatus(payload[1:])
			if err != nil {
				close(stop)
				return 0, err
			}
			exitCode, gotStatus = code, true
		case 255:
			if len(payload) != 2 {
				close(stop)
				return 0, ErrBoundary
			}
		default:
			close(stop)
			return 0, ErrBoundary
		}
	}
	close(stop)
	if !writerCompleted {
		select {
		case writerErr = <-writerDone:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if writerErr != nil && !errors.Is(writerErr, context.Canceled) {
		return exitCode, errors.Join(ErrTransport, writerErr)
	}
	if !gotStatus {
		return 0, ErrTransport
	}
	return exitCode, nil
}

func writeExecInput(ctx context.Context, conn *websocket.Conn, input io.Reader, stop <-chan struct{}) error {
	if input == nil {
		input = strings.NewReader("")
	}
	buffer := make([]byte, 4095)
	total := 0
	for {
		select {
		case <-stop:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := input.Read(buffer)
		if n > 0 {
			total += n
			if total > 96<<20 {
				return ErrBoundary
			}
			frame := append([]byte{0}, buffer[:n]...)
			if writeErr := conn.WriteMessage(websocket.BinaryMessage, frame); writeErr != nil {
				return writeErr
			}
		}
		if err == io.EOF {
			return conn.WriteMessage(websocket.BinaryMessage, []byte{255, 0})
		}
		if err != nil {
			return err
		}
		if n == 0 {
			select {
			case <-stop:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
		}
	}
}

func (c *HTTPPodClient) validateRef(ref PodRef, requireUID bool) error {
	if ref.Namespace != c.namespace || !namespaceValue.MatchString(ref.Namespace) || len(ref.Name) > 63 || !strings.HasPrefix(ref.Name, "rf-ws-") || requireUID && ref.UID == "" {
		return ErrBoundary
	}
	return nil
}

func (c *HTTPPodClient) resourceURL(parts ...string) string {
	copyURL := c.baseURL
	segments := []string{"namespaces", c.namespace}
	segments = append(segments, parts...)
	copyURL.Path = path.Join(copyURL.Path, path.Join(segments...))
	return copyURL.String()
}

func (c *HTTPPodClient) requestJSON(ctx context.Context, method, target string, body []byte, result any) error {
	responseBody, err := c.request(ctx, method, target, body)
	if err != nil || result == nil {
		return err
	}
	if err = json.Unmarshal(responseBody, result); err != nil {
		return errors.Join(ErrTransport, err)
	}
	return nil
}

func (c *HTTPPodClient) request(ctx context.Context, method, target string, body []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	token, err := c.readToken()
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(string(body)))
	if err != nil {
		return nil, errors.Join(ErrTransport, err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, errors.Join(ErrTransport, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(responseBody) > 1<<20 {
		return nil, errors.Join(ErrTransport, err, ErrBoundary)
	}
	if method == http.MethodDelete && response.StatusCode == http.StatusNotFound {
		return responseBody, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: API status %d: %s", ErrTransport, response.StatusCode, sanitizeAPIError(responseBody))
	}
	return responseBody, nil
}

func parsePod(data []byte) (Pod, error) {
	var response struct {
		Metadata struct {
			Namespace         string            `json:"namespace"`
			Name              string            `json:"name"`
			UID               string            `json:"uid"`
			Labels            map[string]string `json:"labels"`
			CreationTimestamp string            `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			ActiveDeadlineSeconds int64 `json:"activeDeadlineSeconds"`
		} `json:"spec"`
		Status struct {
			Phase      string `json:"phase"`
			Reason     string `json:"reason"`
			Message    string `json:"message"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
				Reason string `json:"reason"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return Pod{}, errors.Join(ErrTransport, err)
	}
	if response.Metadata.Namespace == "" || response.Metadata.Name == "" || response.Metadata.UID == "" {
		return Pod{}, ErrBoundary
	}
	var createdAt time.Time
	if response.Metadata.CreationTimestamp != "" {
		var parseErr error
		createdAt, parseErr = time.Parse(time.RFC3339Nano, response.Metadata.CreationTimestamp)
		if parseErr != nil {
			return Pod{}, ErrBoundary
		}
	}
	unschedulable := false
	for _, condition := range response.Status.Conditions {
		unschedulable = unschedulable || condition.Type == "PodScheduled" && condition.Status == "False" && condition.Reason == "Unschedulable"
	}
	return Pod{Ref: PodRef{Namespace: response.Metadata.Namespace, Name: response.Metadata.Name, UID: response.Metadata.UID}, Phase: response.Status.Phase, Reason: response.Status.Reason, Message: response.Status.Message, CreatedAt: createdAt, ActiveDeadlineSeconds: response.Spec.ActiveDeadlineSeconds, Labels: response.Metadata.Labels, Unschedulable: unschedulable}, nil
}

func (c *HTTPPodClient) readToken() (string, error) {
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return "", errors.Join(ErrTransport, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return "", ErrBoundary
	}
	return token, nil
}

func sanitizeAPIError(data []byte) string {
	var status struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &status) == nil {
		message := strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return ' '
			}
			return r
		}, status.Message)
		if len(message) > 512 {
			message = message[:512]
		}
		return status.Reason + " " + message
	}
	return "invalid API error response"
}

func makePodBody(spec PodSpec) map[string]any {
	labels := make(map[string]string, len(spec.Labels))
	for key, value := range spec.Labels {
		labels[key] = value
	}
	env := make([]map[string]string, 0, len(spec.Environment))
	for _, value := range spec.Environment {
		key, val, _ := strings.Cut(value, "=")
		env = append(env, map[string]string{"name": key, "value": val})
	}
	resources := map[string]any{
		"requests": map[string]string{"cpu": strconv.FormatInt(spec.Resources.CPUs*1000, 10) + "m", "memory": strconv.FormatInt(spec.Resources.MemoryBytes, 10)},
		"limits":   map[string]string{"cpu": strconv.FormatInt(spec.Resources.CPUs*1000, 10) + "m", "memory": strconv.FormatInt(spec.Resources.MemoryBytes, 10)},
	}
	container := map[string]any{
		"name": spec.ContainerName, "image": spec.Image, "workingDir": spec.WorkingDirectory, "stdin": false, "tty": false, "env": env,
		"securityContext": map[string]any{"runAsUser": spec.RunAsUser, "runAsGroup": spec.RunAsGroup, "runAsNonRoot": spec.RunAsNonRoot, "allowPrivilegeEscalation": spec.AllowPrivilegeEscalation, "readOnlyRootFilesystem": spec.ReadOnlyRootFilesystem, "capabilities": map[string][]string{"drop": spec.DropCapabilities}},
		"resources":       resources,
		"volumeMounts":    []map[string]any{{"name": "workspace", "mountPath": "/workspace"}, {"name": "tmp", "mountPath": "/tmp"}},
	}
	volumes := []map[string]any{{"name": "workspace", "emptyDir": map[string]string{"sizeLimit": strconv.FormatInt(spec.WorkspaceEmptyDirBytes, 10)}}, {"name": "tmp", "emptyDir": map[string]string{"sizeLimit": strconv.FormatInt(spec.TempEmptyDirBytes, 10)}}}
	if spec.CacheClaim != "" {
		container["volumeMounts"] = append(container["volumeMounts"].([]map[string]any), map[string]any{"name": "cache", "mountPath": "/cache"})
		volumes = append(volumes, map[string]any{"name": "cache", "persistentVolumeClaim": map[string]string{"claimName": spec.CacheClaim}})
	}
	podSpec := map[string]any{
		"automountServiceAccountToken": false, "restartPolicy": "Never", "activeDeadlineSeconds": spec.ActiveDeadlineSeconds,
		"securityContext": map[string]any{"runAsUser": spec.RunAsUser, "runAsGroup": spec.RunAsGroup, "runAsNonRoot": spec.RunAsNonRoot, "fsGroup": spec.FSGroup, "fsGroupChangePolicy": "OnRootMismatch", "seccompProfile": map[string]string{"type": spec.SeccompProfile}},
		"containers":      []any{container},
		"volumes":         volumes,
	}
	if len(spec.ImagePullSecrets) > 0 {
		secrets := make([]map[string]string, 0, len(spec.ImagePullSecrets))
		for _, name := range spec.ImagePullSecrets {
			secrets = append(secrets, map[string]string{"name": name})
		}
		podSpec["imagePullSecrets"] = secrets
	}
	if spec.RuntimeClassName != "" {
		podSpec["runtimeClassName"] = spec.RuntimeClassName
	}
	return map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": spec.Ref.Name, "namespace": spec.Ref.Namespace, "labels": labels}, "spec": podSpec}
}

func parseExecStatus(data []byte) (int, error) {
	if len(data) == 0 || len(data) > 64<<10 {
		return 0, ErrBoundary
	}
	var status struct {
		Status  string `json:"status"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
		Details struct {
			Causes []struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			} `json:"causes"`
		} `json:"details"`
	}
	if json.Unmarshal(data, &status) != nil || status.Status == "" {
		return 0, ErrBoundary
	}
	if status.Status == "Success" {
		return 0, nil
	}
	if status.Status != "Failure" {
		return 0, ErrBoundary
	}
	for _, cause := range status.Details.Causes {
		if cause.Reason == "ExitCode" {
			code, err := strconv.Atoi(cause.Message)
			if err == nil && code >= 0 && code <= 255 {
				return code, nil
			}
			return 0, ErrBoundary
		}
	}
	return 0, fmt.Errorf("%w: %s", ErrTransport, sanitizeAPIError(data))
}

type execOutput struct {
	stdout io.Writer
	stderr io.Writer
	limit  int
	used   int
}

func (o *execOutput) Write(channel byte, data []byte) error {
	if len(data) > o.limit-o.used {
		return ErrOutputLimit
	}
	o.used += len(data)
	writer := o.stdout
	if channel == 2 {
		writer = o.stderr
	}
	if writer != nil {
		if _, err := writer.Write(data); err != nil {
			return err
		}
	}
	return nil
}
