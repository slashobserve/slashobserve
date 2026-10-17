package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/slashobserve/slashobserve/demo/shop/internal/shopapp"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) != 1 {
		return usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var err error
	switch args[0] {
	case "serve":
		err = shopapp.Serve(ctx, os.LookupEnv)
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

func usage() int {
	fmt.Fprintln(os.Stderr, "usage: shop serve|healthcheck")
	return 2
}
