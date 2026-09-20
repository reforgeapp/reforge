package main

import (
	"fmt"
	"os"
	"reforge/internal/sandbox/guest"
)

func main() {
	if err := guest.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sandbox file operation failed")
		os.Exit(1)
	}
}
