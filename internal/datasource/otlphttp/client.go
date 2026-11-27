package otlphttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	defaultMaxResponseBytes = 64 << 20
	maxConfigurableBytes    = 1 << 30
)

var reservedHeaders = []string{"Accept", "Host", "Content-Length"}

type Options struct {
	URL               string
	Headers           map[string]string
	BasicAuthUser     string
	BasicAuthPassword datasource.Secret
	BearerToken       datasource.Secret
	Limits            datasource.Limits
	HTTPClient        *http.Client
	MaxResponseBytes  int64
}

type Client struct {
	opts       Options
	prefix     string
	client     *http.Client
	ownsClient bool
}

func New(kind string, opts Options) (*Client, error) {
	prefix, err := parseBaseURL(kind, opts.URL)
	if err != nil {
		return nil, err
	}
	switch {
	case opts.MaxResponseBytes < 0:
		return nil, datasource.ConfigErrorf("%s: max response bytes must not be negative", kind)
	case opts.MaxResponseBytes > maxConfigurableBytes:
		return nil, datasource.ConfigErrorf("%s: max response bytes must not exceed %d", kind, maxConfigurableBytes)
	case opts.MaxResponseBytes == 0:
		opts.MaxResponseBytes = defaultMaxResponseBytes
	}
	opts.Limits = opts.Limits.WithDefaults(datasource.DefaultLimits)
	if err := opts.Limits.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	if err := validateHeaders(kind, opts); err != nil {
		return nil, err
	}
	refuseRedirects := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := &Client{opts: opts, prefix: prefix}
	if opts.HTTPClient == nil {
		c.client = &http.Client{
			Transport:     datasource.NewHTTPTransport(),
			CheckRedirect: refuseRedirects,
		}
		c.ownsClient = true
	} else {
		supplied := *opts.HTTPClient
		supplied.CheckRedirect = refuseRedirects
		c.client = &supplied
	}
	return c, nil
}

func validateHeaders(kind string, opts Options) error {
	if opts.BasicAuthUser == "" && !opts.BasicAuthPassword.IsZero() {
		return datasource.ConfigErrorf("%s: basic auth password requires a user", kind)
	}
	if opts.BasicAuthUser != "" && !opts.BearerToken.IsZero() {
		return datasource.ConfigErrorf("%s: basic auth and bearer token are mutually exclusive", kind)
	}
	for name, value := range opts.Headers {
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return datasource.ConfigErrorf("%s: invalid header", kind)
		}
		switch {
		case strings.EqualFold(name, "Authorization"):
			return datasource.ConfigErrorf("%s: the Authorization header is set through basic auth or bearer token", kind)
		case slices.ContainsFunc(reservedHeaders, func(r string) bool { return strings.EqualFold(name, r) }):
			return datasource.ConfigErrorf("%s: the %s header is managed by the datasource", kind, http.CanonicalHeaderKey(name))
		}
	}
	return nil
}

func parseBaseURL(kind, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", datasource.ConfigErrorf("%s: invalid url", kind)
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", datasource.ConfigErrorf("%s: url scheme must be http or https", kind)
	case u.Hostname() == "":
		return "", datasource.ConfigErrorf("%s: url requires a host", kind)
	case u.User != nil:
		return "", datasource.ConfigErrorf("%s: url must not contain user info", kind)
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", datasource.ConfigErrorf("%s: url must not contain a query or fragment", kind)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (c *Client) Options() Options { return c.opts }

func (c *Client) Close() error {
	if c.ownsClient {
		c.client.CloseIdleConnections()
	}
	return nil
}

func (c *Client) Fetch(ctx context.Context, path string, params url.Values) (int, []byte, error) {
	target := c.prefix + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, nil, datasource.Errorf(datasource.ErrUnavailable, "connection failed")
	}
	req.Header.Set("Accept", "application/json")
	for name, value := range c.opts.Headers {
		req.Header.Set(name, value)
	}
	switch {
	case c.opts.BasicAuthUser != "":
		req.SetBasicAuth(c.opts.BasicAuthUser, c.opts.BasicAuthPassword.Reveal())
	case !c.opts.BearerToken.IsZero():
		req.Header.Set("Authorization", "Bearer "+c.opts.BearerToken.Reveal())
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, datasource.TransportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.opts.MaxResponseBytes+1))
	if err != nil {
		return 0, nil, datasource.TransportError(ctx, err)
	}
	if int64(len(raw)) > c.opts.MaxResponseBytes {
		return 0, nil, datasource.Errorf(datasource.ErrLimitExceeded, "response larger than %d bytes", c.opts.MaxResponseBytes)
	}
	return resp.StatusCode, raw, nil
}
