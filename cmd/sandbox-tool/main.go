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
	if len(os.Args) == 2 && os.Args[1] == "validate-bot-config" {
		problems := guest.ValidateBotConfig(os.DirFS("."))
		for _, problem := range problems {
			fmt.Println(problem)
		}
		if len(problems) > 0 {
			os.Exit(1)
		}
		fmt.Println("bot configuration valid")
		return
	}
	if err := guest.Run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "sandbox file operation failed")
		os.Exit(1)
	}
}
