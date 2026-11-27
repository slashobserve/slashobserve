package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	root := flag.String("root", "", "directory to serve, read-only")
	addr := flag.String("addr", listenDefault(), "listen address, default from CODESERVER_ADDR or 127.0.0.1:8080")
	healthcheck := flag.Bool("healthcheck", false, "probe the server on -addr and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(probe(*addr))
	}
	if *root == "" {
		fmt.Fprintln(os.Stderr, "codeserver: -root is required")
		os.Exit(2)
	}
	if err := run(*root, *addr); err != nil {
		slog.Error("codeserver stopped", "cause", err.Error())
		os.Exit(1)
	}
}

func listenDefault() string {
	if v := os.Getenv("CODESERVER_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:8080"
}

func run(dir, addr string) error {
	root, err := NewRoot(dir)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           NewHandler(&Code{root: root}, version),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()
	slog.Info("codeserver listening", "addr", addr, "root", root.dir)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func probe(addr string) int {
	host := addr
	if len(host) > 0 && host[0] == ':' {
		host = "127.0.0.1" + host
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + host + healthPath)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return 1
	}
	return 0
}
