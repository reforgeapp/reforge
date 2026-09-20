package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "inspect":
		if os.Getenv("REFORGE_SANDBOX_CANARY") != "" {
			panic("host environment exposed")
		}
		for _, path := range []string{"/home/mnorris/.ssh", "/var/run/docker.sock", "/host/etc/passwd"} {
			if _, err := os.Stat(path); err == nil {
				panic("host path exposed")
			}
		}
		for _, address := range []string{"127.0.0.1:55432", "169.254.169.254:80"} {
			c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
			if err == nil {
				c.Close()
				panic("network exposed")
			}
		}
		fmt.Println("host paths, environment and network isolated")
	case "symlink":
		if err := os.Symlink("/etc/passwd", "/workspace/escape"); err != nil {
			panic(err)
		}
	case "fifo":
		if err := syscall.Mkfifo("/workspace/fifo", 0600); err != nil {
			panic(err)
		}
	case "readonly":
		if err := os.WriteFile("/bin/injected", []byte("bad"), 0600); err == nil {
			panic("image writable")
		}
		if err := os.WriteFile("/opt/reforge/tool", []byte("bad"), 0600); err == nil {
			panic("tool writable")
		}
		fmt.Println("readonly")
	case "disk":
		chunk := make([]byte, 1<<20)
		for i := 0; i < 32; i++ {
			f, err := os.Create(fmt.Sprintf("/workspace/fill-%d", i))
			if err == nil {
				_, err = f.Write(chunk)
				_ = f.Close()
			}
			if err != nil {
				fmt.Println("aggregate disk ceiling enforced")
				return
			}
		}
		panic("aggregate disk ceiling absent")
	case "hang":
		cmd := exec.Command("/bin/probe", "descendant")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		fmt.Println("descendant-started")
		for {
			time.Sleep(time.Second)
		}
	case "descendant":
		for {
			_ = os.WriteFile("/workspace/alive", []byte("alive"), 0600)
			time.Sleep(10 * time.Millisecond)
		}
	case "flood":
		os.Stdout.Write(bytes.Repeat([]byte("x"), 2<<20))
	case "read":
		data, err := os.ReadFile("/workspace/source.txt")
		if err != nil {
			panic(err)
		}
		os.Stdout.Write(data)
	default:
		os.Exit(2)
	}
}
