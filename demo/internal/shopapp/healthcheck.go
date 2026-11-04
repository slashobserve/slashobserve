package shopapp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const healthcheckTimeout = 2 * time.Second

func Healthcheck(ctx context.Context, lookup func(string) (string, bool)) error {
	addr := listenAddr(lookup)
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse SHOP_ADDR %q: %w", addr, err)
	}
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", port)+"/healthz", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %s", resp.Status)
	}
	return nil
}
