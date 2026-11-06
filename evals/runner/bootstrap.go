package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	devLoginUser     = "admin@example.com"
	devLoginPassword = "password"
	tokenPrefix      = "sok_"
)

var loopbackHosts = []string{"localhost", "127.0.0.1"}

func checkLoopback(server string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("--bootstrap-dex needs --server to be an http or https URL")
	}
	for _, host := range loopbackHosts {
		if u.Hostname() == host {
			return u, nil
		}
	}
	return nil, fmt.Errorf("--bootstrap-dex signs in with the demo stack's fixed development login, which exists only on %s, not on %q", strings.Join(loopbackHosts, " or "), u.Hostname())
}

type bootstrapper struct {
	base *url.URL
	http *http.Client
	now  func() time.Time
}

func newBootstrapper(base *url.URL) (*bootstrapper, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &bootstrapper{base: base, http: &http.Client{Jar: jar, Timeout: callTimeout}, now: time.Now}, nil
}

func bootstrapKey(ctx context.Context, server string) (string, error) {
	base, err := checkLoopback(server)
	if err != nil {
		return "", err
	}
	b, err := newBootstrapper(base)
	if err != nil {
		return "", err
	}
	return b.key(ctx)
}

func (b *bootstrapper) key(ctx context.Context) (string, error) {
	if err := b.login(ctx); err != nil {
		return "", fmt.Errorf("sign in through Dex: %w", err)
	}
	var account struct {
		ID string `json:"id"`
	}
	name := fmt.Sprintf("evals-%d", b.now().Unix())
	if err := b.post(ctx, "/api/v1/service-accounts", map[string]string{"name": name}, &account); err != nil {
		return "", fmt.Errorf("create service account %s: %w", name, err)
	}
	if account.ID == "" {
		return "", errors.New("the service account response has no id")
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := b.post(ctx, "/api/v1/service-accounts/"+url.PathEscape(account.ID)+"/keys", map[string]string{"name": name}, &issued); err != nil {
		return "", fmt.Errorf("create a key for service account %s: %w", name, err)
	}
	if !strings.HasPrefix(issued.Token, tokenPrefix) {
		return "", errors.New("the key response has no token")
	}
	return issued.Token, nil
}

func (b *bootstrapper) login(ctx context.Context) error {
	landing := b.base.String() + "/"
	authorize := b.base.String() + "/api/v1/auth/login?" + url.Values{"return_to": {"/"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authorize, nil)
	if err != nil {
		return err
	}
	form, err := b.http.Do(req)
	if err != nil {
		return err
	}
	page, err := io.ReadAll(io.LimitReader(form.Body, maxBodyBytes))
	_ = form.Body.Close()
	if err != nil {
		return err
	}
	if !bytes.Contains(page, []byte(`name="login"`)) {
		return fmt.Errorf("the Dex login form was not reached (landed on %s, status %d)", form.Request.URL.Redacted(), form.StatusCode)
	}
	creds := url.Values{"login": {devLoginUser}, "password": {devLoginPassword}}
	post, err := http.NewRequestWithContext(ctx, http.MethodPost, form.Request.URL.String(), strings.NewReader(creds.Encode()))
	if err != nil {
		return err
	}
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	final, err := b.http.Do(post)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(final.Body, maxBodyBytes))
	_ = final.Body.Close()
	if got := final.Request.URL.String(); got != landing {
		return fmt.Errorf("the login code flow landed on %s, want %s; pass --server as the stack's public URL", final.Request.URL.Redacted(), landing)
	}
	return nil
}

func (b *bootstrapper) post(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base.String()+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Origin", b.base.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return decodeAPIError(resp.StatusCode, raw)
	}
	return json.Unmarshal(raw, out)
}
