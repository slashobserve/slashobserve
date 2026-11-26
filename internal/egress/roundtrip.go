package egress

import (
	"net/http"
	"time"
)

const callHeaderTimeout = 60 * time.Second

type callTripper struct {
	c         *Client
	transport *http.Transport
}

func (c *Client) CallTripper() http.RoundTripper { return callTripper{c: c, transport: c.calls} }

func (c *Client) PrivateCallTripper() http.RoundTripper {
	return callTripper{c: c, transport: c.privateCalls}
}

func (t callTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	host, err := anyPortHostOf(req.URL)
	if err != nil {
		return nil, err
	}
	list, err := t.c.allow.Allowed(req.Context())
	if err != nil {
		return nil, err
	}
	if !list.Allows(host) {
		return nil, &HostError{Host: host, Err: ErrNotAllowed}
	}
	out := req.Clone(req.Context())
	if out.Header.Get("User-Agent") == "" {
		out.Header.Set("User-Agent", t.c.userAgent)
	}
	return t.transport.RoundTrip(out)
}
