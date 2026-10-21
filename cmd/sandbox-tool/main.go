package main

import (
	"fmt"
	"github.com/reforgeapp/reforge/pkg/sandbox/guest"
	"io/fs"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "egress" {
		os.Exit(guest.Egress(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "egress-tcp" && len(os.Args) == 2 {
		os.Exit(guest.EgressTCP())
	}
	validators := map[string]func(fs.FS) []string{"validate-bot-config": guest.ValidateBotConfig, "validate-bootstrap": guest.ValidateBootstrap, "validate-docs": guest.ValidateDocs, "validate-none": func(fs.FS) []string { return nil }}
	if validate := validators[os.Args[len(os.Args)-1]]; len(os.Args) == 2 && validate != nil {
		problems := validate(os.DirFS("."))
		for _, problem := range problems {
			fmt.Println(problem)
		}
		if len(problems) > 0 {
			os.Exit(1)
		}
		if os.Args[1] == "validate-none" {
			fmt.Println("no repository tests; proof comes from native CI and post-merge observation")
			return
		}
		fmt.Println("validation passed")
		return
	}
	if err := guest.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sandbox file operation failed:", err)
		os.Exit(1)
	}
}
