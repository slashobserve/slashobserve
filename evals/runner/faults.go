package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	faultClearTimeout = 10 * time.Second

	defaultShopPort  = "28081"
	defaultShop1Port = "28082"
)

type shopURLs map[string]string

func (s *shopURLs) String() string {
	if s == nil {
		return ""
	}
	pairs := make([]string, 0, len(*s))
	for pod, base := range *s {
		pairs = append(pairs, pod+"="+base)
	}
	return strings.Join(pairs, ",")
}

func (s *shopURLs) Set(v string) error {
	pod, base, ok := strings.Cut(v, "=")
	u, err := url.Parse(base)
	if !ok || blank(pod) || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("--shop-url %q must be pod=http://host:port", v)
	}
	if *s == nil {
		*s = shopURLs{}
	}
	(*s)[pod] = strings.TrimRight(base, "/")
	return nil
}

func resolveShopURLs(flagged shopURLs, getenv func(string) string) map[string]string {
	port := func(name, fallback string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return fallback
	}
	urls := map[string]string{
		"checkout-0": "http://127.0.0.1:" + port("SLASHOBSERVE_EVALS_SHOP_PORT", defaultShopPort),
		"checkout-1": "http://127.0.0.1:" + port("SLASHOBSERVE_EVALS_SHOP1_PORT", defaultShop1Port),
	}
	for pod, base := range flagged {
		urls[pod] = base
	}
	return urls
}

type shopFaults struct {
	urls     map[string]string
	http     *http.Client
	soakUnit time.Duration

	one sync.Mutex
}

func newShopFaults(urls map[string]string) *shopFaults {
	return &shopFaults{urls: urls, http: &http.Client{Timeout: callTimeout}, soakUnit: time.Second}
}

func (s *shopFaults) withFault(ctx context.Context, f Fault, fn func(context.Context) error) (err error) {
	base, ok := s.urls[f.Pod]
	if !ok {
		return fmt.Errorf("no --shop-url for pod %q", f.Pod)
	}
	s.one.Lock()
	defer s.one.Unlock()
	defer func() {
		clearCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), faultClearTimeout)
		defer cancel()
		if clearErr := s.call(clearCtx, http.MethodDelete, base+"/fault"); clearErr != nil {
			err = errors.Join(err, fmt.Errorf("clear the fault on %s: %w", f.Pod, clearErr))
		}
	}()
	query := url.Values{"type": {f.Type}, "pct": {strconv.Itoa(f.Pct)}}
	if err := s.call(ctx, http.MethodPost, base+"/fault?"+query.Encode()); err != nil {
		return fmt.Errorf("set the %s fault on %s: %w", f.Type, f.Pod, err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(f.SoakSeconds) * s.soakUnit):
	}
	return fn(ctx)
}

func (s *shopFaults) call(ctx context.Context, method, target string) error {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}
