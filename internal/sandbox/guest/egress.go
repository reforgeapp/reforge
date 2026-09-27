package guest

import (
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
)

const EgressSocket = "/run/reforge/egress.sock"

func Egress(command []string) int {
	env := os.Environ()
	for len(command) > 0 && strings.Contains(command[0], "=") && !strings.HasPrefix(command[0], "/") {
		env = append(env, command[0])
		command = command[1:]
	}
	if len(command) == 0 {
		return 2
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 2
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go forward(conn)
		}
	}()
	proxy := "http://" + listener.Addr().String()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = append(env, "HTTPS_PROXY="+proxy, "https_proxy="+proxy, "HTTP_PROXY="+proxy, "http_proxy="+proxy, "npm_config_https_proxy="+proxy, "npm_config_proxy="+proxy, "NO_PROXY=", "no_proxy=")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	return exitCode(err)
}

func forward(client net.Conn) {
	defer client.Close()
	upstream, err := net.Dial("unix", EgressSocket)
	if err != nil {
		return
	}
	defer upstream.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
}
