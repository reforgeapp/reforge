package guest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestForwardTCPAuthenticatesCONNECTAndBridgesTunnel(t *testing.T) {
	broker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	address := broker.Addr().String()
	token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	brokerDone := make(chan error, 1)
	go func() {
		conn, err := broker.Accept()
		if err != nil {
			brokerDone <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		request, err := http.ReadRequest(reader)
		if err != nil {
			brokerDone <- err
			return
		}
		if request.Method != http.MethodConnect || request.Host != "registry.npmjs.org:443" || request.Header.Get("Proxy-Authorization") != "Bearer "+token {
			brokerDone <- fmt.Errorf("unexpected broker request: method=%q host=%q", request.Method, request.Host)
			return
		}
		if _, err = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\nREADY"); err != nil {
			brokerDone <- err
			return
		}
		payload := make([]byte, 4)
		if _, err = io.ReadFull(reader, payload); err != nil {
			brokerDone <- err
			return
		}
		if string(payload) != "PING" {
			brokerDone <- fmt.Errorf("payload=%q", payload)
			return
		}
		_, err = conn.Write(payload)
		brokerDone <- err
	}()

	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	go func() {
		conn, err := local.Accept()
		if err == nil {
			forwardTCP(conn, address, token)
		}
	}()
	client, err := net.DialTimeout("tcp", local.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	reader := bufio.NewReader(client)
	if _, err = io.WriteString(client, "CONNECT registry.npmjs.org:443 HTTP/1.1\r\nHost: registry.npmjs.org:443\r\nProxy-Authorization: Bearer attacker\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	_ = client.SetDeadline(time.Now().Add(time.Second))
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("response=%v error=%v", response, err)
	}
	banner := make([]byte, 5)
	if _, err = io.ReadFull(reader, banner); err != nil || string(banner) != "READY" {
		t.Fatalf("banner=%q error=%v", banner, err)
	}
	if _, err = io.WriteString(client, "PING"); err != nil {
		t.Fatal(err)
	}
	pong := make([]byte, 4)
	if _, err = io.ReadFull(reader, pong); err != nil || string(pong) != "PING" {
		t.Fatalf("pong=%q error=%v", pong, err)
	}
	if err = <-brokerDone; err != nil {
		t.Fatal(err)
	}
}

func TestTCPConfigRequiresLiteralAddressAndRandomToken(t *testing.T) {
	token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	command := []string{"npm", "ci"}
	if !validTCPEgressRequest(TCPEgressRequest{Address: netip.AddrPortFrom(netip.MustParseAddr("10.0.0.2"), 8443).String(), Token: token, Command: command}) {
		t.Fatal("valid TCP egress request rejected")
	}
	for _, address := range []string{"runner-proxy:8443", "10.0.0.2:0", "[fe80::1%eth0]:8443"} {
		if validTCPEgressRequest(TCPEgressRequest{Address: address, Token: token, Command: command}) {
			t.Fatalf("invalid broker address accepted: %q", address)
		}
	}
}

func TestEgressTCPSubprocessHelper(t *testing.T) {
	if os.Getenv("REFORGE_EGRESS_TCP_HELPER") != "1" {
		return
	}
	os.Exit(EgressTCP())
}

func TestEgressTCPForwardsOutputAndKeepsConfigFromCommandStdin(t *testing.T) {
	config, err := json.Marshal(TCPEgressRequest{
		Address: "127.0.0.1:9",
		Token:   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Command: []string{"/bin/sh", "-c", "if read -r value; then echo config-leaked; else echo stdin-empty; fi; echo stdout-visible; echo stderr-visible >&2; exit 7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestEgressTCPSubprocessHelper$")
	command.Env = append(os.Environ(), "REFORGE_EGRESS_TCP_HELPER=1")
	command.Stdin = bytes.NewReader(config)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("subprocess error=%v", err)
	}
	if !strings.Contains(stdout.String(), "stdin-empty") || strings.Contains(stdout.String(), "config-leaked") || !strings.Contains(stdout.String(), "stdout-visible") || !strings.Contains(stderr.String(), "stderr-visible") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
