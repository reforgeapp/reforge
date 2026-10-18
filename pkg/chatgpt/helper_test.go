package chatgpt

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLocalHelperRejectsUnboundCallbacksAndExits(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 unavailable")
	}
	state := Random()
	authorization := Issuer + "/api/accounts/authorize?" + url.Values{"redirect_uri": {LocalRedirect}, "state": {state}}.Encode()
	callback := "https://app.example/auth/openai/callback"
	helper, err := LocalHelper(authorization, callback)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	raw, _ := json.Marshal(helper)
	command := exec.CommandContext(ctx, python, "-u", "-c", "import webbrowser; webbrowser.open = lambda url: True; exec("+string(raw)+")")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || !strings.Contains(line, "Complete ChatGPT sign-in") {
		t.Fatalf("callback listener failed to start: %s %v", line, err)
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	for _, query := range []string{"state=wrong", "state=%C3%A9", "state=" + state + "&state=duplicate", "state=" + state + "&code=one&code=two"} {
		response, err := client.Get(LocalRedirect + "?" + query)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 || response.Header.Get("Location") != "" {
			t.Fatal("unbound callback forwarded")
		}
	}
	query := url.Values{"state": {state}, "code": {"fixture-code"}, "client_id": {"oaiapp_fixture"}, "access_token": {"must-not-forward"}}
	response, err := client.Get(LocalRedirect + "?" + query.Encode())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || response.StatusCode != 303 || location.Scheme != "https" || location.Host != "app.example" || location.Query().Get("state") != state || location.Query().Get("code") != "fixture-code" || location.Query().Get("client_id") != "oaiapp_fixture" || location.Query().Has("access_token") || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("callback lost binding or leaked unexpected credentials")
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", "127.0.0.1:1456", time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("callback listener remained running")
	}
	for _, callback := range []string{"http://app.example/auth/openai/callback", "https://app.example/redirect", "https://owner@app.example/auth/openai/callback"} {
		if _, err := LocalHelper(authorization, callback); err == nil {
			t.Fatal("unsafe callback target accepted")
		}
	}
}
