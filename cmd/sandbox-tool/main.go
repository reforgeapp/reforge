package main

import (
	"fmt"
	"io/fs"
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
	validators := map[string]func(fs.FS) []string{"validate-bot-config": guest.ValidateBotConfig, "validate-bootstrap": guest.ValidateBootstrap, "validate-docs": guest.ValidateDocs}
	if validate := validators[os.Args[len(os.Args)-1]]; len(os.Args) == 2 && validate != nil {
		problems := validate(os.DirFS("."))
		for _, problem := range problems {
			fmt.Println(problem)
		}
		if len(problems) > 0 {
			os.Exit(1)
		}
		fmt.Println("validation passed")
		return
	}
	if err := guest.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sandbox file operation failed")
		os.Exit(1)
	}
}
