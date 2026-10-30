package egress

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const maxSendBytes = 64 << 10

var withheldSendHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie", "Referer"}

type SendRequest struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

type SendResult struct {
	Status int
}

func (c *Client) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return SendResult{}, ErrInvalidURL
	}
	host, err := hostOf(u)
	if err != nil {
		return SendResult{}, err
	}
	list, err := c.allow.Allowed(ctx)
	if err != nil {
		return SendResult{}, fmt.Errorf("read outbound allowlist: %w", err)
	}
	if !list.Allows(host) {
		return SendResult{}, &HostError{Host: host, Err: ErrNotAllowed}
	}
	if len(req.Body) > maxSendBytes {
		return SendResult{}, &HostError{Host: host, Err: ErrTooLarge}
	}
	t := &trail{list: list, hosts: []string{host}}

	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return SendResult{}, ctx.Err()
	}

	client := &http.Client{
		Transport:     c.transport,
		Timeout:       fetchTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	hreq, err := http.NewRequestWithContext(ctx, req.Method, u.String(), body)
	if err != nil {
		return SendResult{}, t.fail(err)
	}
	for name, values := range req.Header {
		for _, value := range values {
			hreq.Header.Add(name, value)
		}
	}
	for _, name := range withheldSendHeaders {
		hreq.Header.Del(name)
	}
	hreq.Header.Set("User-Agent", c.userAgent)

	resp, err := client.Do(hreq)
	if err != nil {
		return SendResult{}, t.fail(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxSendBytes))
	return SendResult{Status: resp.StatusCode}, nil
}
