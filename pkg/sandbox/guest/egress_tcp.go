package guest

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"
)

type TCPEgressRequest struct {
	Address string   `json:"address"`
	Token   string   `json:"token"`
	Command []string `json:"command"`
}

func EgressTCP() int {
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20))
	decoder.DisallowUnknownFields()
	var request TCPEgressRequest
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !validTCPEgressRequest(request) {
		return 2
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 2
	}
	defer listener.Close()
	proxy := "http://" + listener.Addr().String()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go forwardTCP(conn, request.Address, request.Token)
		}
	}()
	command := exec.Command(request.Command[0], request.Command[1:]...)
	command.Env = append(os.Environ(), "HTTPS_PROXY="+proxy, "https_proxy="+proxy, "HTTP_PROXY="+proxy, "http_proxy="+proxy, "npm_config_https_proxy="+proxy, "npm_config_proxy="+proxy, "NO_PROXY=", "no_proxy=")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return exitCode(command.Run())
}

func validTCPEgressRequest(request TCPEgressRequest) bool {
	address, err := netip.ParseAddrPort(request.Address)
	if err != nil || address.Addr().Zone() != "" || address.Port() == 0 || !address.Addr().IsValid() || len(request.Command) == 0 || len(request.Command) > 128 {
		return false
	}
	token, err := base64.RawURLEncoding.DecodeString(request.Token)
	if err != nil || len(token) != 32 || len(request.Token) > 128 {
		return false
	}
	for _, arg := range request.Command {
		if len(arg) > 16384 || strings.ContainsRune(arg, 0) {
			return false
		}
	}
	return true
}

func forwardTCP(client net.Conn, address, token string) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))
	reader := bufio.NewReader(client)
	request, err := http.ReadRequest(reader)
	if err != nil || request.Method != http.MethodConnect || request.Host == "" || strings.ContainsAny(request.Host, "\r\n") {
		return
	}
	upstream, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()
	if _, err = io.WriteString(upstream, "CONNECT "+request.Host+" HTTP/1.1\r\nHost: "+request.Host+"\r\nProxy-Authorization: Bearer "+token+"\r\n\r\n"); err != nil {
		return
	}
	upstreamReader := bufio.NewReader(upstream)
	response, err := http.ReadResponse(upstreamReader, request)
	if err != nil {
		return
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Write(client)
		return
	}
	if _, err = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, reader); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstreamReader); done <- struct{}{} }()
	<-done
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 127
}
