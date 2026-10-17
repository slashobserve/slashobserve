package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/slashobserve/slashobserve/internal/config"
)

const healthcheckTimeout = 3 * time.Second

var healthcheckClient = &http.Client{Transport: &http.Transport{Proxy: nil}}

func healthcheck(lookup func(string) (string, bool)) int {
	addr, err := config.HTTPAddr(lookup)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	target, err := readinessURL(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp, err := healthcheckClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "readyz returned %s\n", resp.Status)
		return 1
	}
	return 0
}

func readinessURL(listenAddr string) (string, error) {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return "", fmt.Errorf("parse listen address %q: %w", listenAddr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/readyz", nil
}
