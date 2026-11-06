package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/slashobserve/slashobserve/demo/internal/keygen"
	"github.com/slashobserve/slashobserve/demo/internal/loadgen"
	"github.com/slashobserve/slashobserve/demo/internal/shopapp"
	"github.com/slashobserve/slashobserve/demo/internal/synth"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 2 && args[0] == "keygen" {
		return keygenAt(args[1])
	}
	if len(args) != 1 {
		return usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var err error
	switch args[0] {
	case "serve":
		err = shopapp.Serve(ctx, os.LookupEnv)
	case "loadgen":
		err = loadgen.Run(ctx, os.LookupEnv, slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	case "synth":
		err = synth.Run(ctx, os.LookupEnv)
	case "healthcheck":
		err = shopapp.Healthcheck(ctx, os.LookupEnv)
	default:
		return usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "shop:", err)
		return 1
	}
	return 0
}

func keygenAt(path string) int {
	outcome, err := keygen.Run(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shop:", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "shop: demo key", outcome, "at", path)
	return 0
}

func usage() int {
	fmt.Fprintln(os.Stderr, "usage: shop serve|loadgen|synth|healthcheck|keygen <path>")
	return 2
}
