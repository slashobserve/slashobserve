package main

import (
	"os"

	"github.com/slashobserve/slashobserve/internal/cli"
)

var version = "dev"

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}, version))
}
