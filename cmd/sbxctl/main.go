package main

import (
	"os"

	"github.com/legibet/sbxctl/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
