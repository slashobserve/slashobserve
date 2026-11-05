package egress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) FetchAuthorized(ctx context.Context, req AuthorizedRequest) ([]byte, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	u.Fragment, u.RawFragment = "", ""
	hostRule, transport := hostOf, c.transport
	if req.AllowPrivate {
		hostRule, transport = anyPortHostOf, c.private
	}
	host, err := hostRule(u)
	if err != nil {
		return nil, err
	}
	list, err := c.allow.Allowed(ctx)
	if err != nil {
		return nil, fmt.Errorf("read outbound allowlist: %w", err)
	}
	if !list.Allows(host) {
		return nil, &HostError{Host: host, Err: ErrNotAllowed}
	}
	j := job{url: u, host: host, accept: req.Accept, maxBytes: clampBytes(req.MaxBytes), list: list}
	t := &trail{list: list, hosts: []string{host}}

	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   fetchTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return &HostError{Host: host, Err: ErrRedirected}
		},
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, t.fail(err)
	}
	hreq.Header.Set("User-Agent", c.userAgent)
	if len(req.Accept) > 0 {
		hreq.Header.Set("Accept", strings.Join(req.Accept, ", "))
	}
	if req.Authorization != "" {
		hreq.Header.Set("Authorization", req.Authorization)
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return nil, t.fail(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxSendBytes))
		return nil, &HostError{Host: host, Err: &StatusError{Status: resp.StatusCode}}
	}
	got, err := readBody(resp, j, t)
	if err != nil {
		return nil, err
	}
	return got.resp.Body, nil
}
