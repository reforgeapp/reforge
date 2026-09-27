package main

import (
	"fmt"
	"os"
	"reforge/internal/sandbox/guest"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "egress" {
		os.Exit(guest.Egress(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "egress-tcp" && len(os.Args) == 2 {
		os.Exit(guest.EgressTCP())
	}
	if err := guest.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sandbox file operation failed")
		os.Exit(1)
	}
}
