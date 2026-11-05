package loki

import (
	"context"
	"fmt"
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
	tenantHeader            = "X-Scope-OrgID"
)

var reservedHeaders = []string{"Accept", "Host", "Content-Length", tenantHeader}

type Options struct {
	URL               string
	Headers           map[string]string
	BasicAuthUser     string
	BasicAuthPassword datasource.Secret
	BearerToken       datasource.Secret
	TenantID          string
	Limits            datasource.Limits
	HTTPClient        *http.Client
	MaxResponseBytes  int64
}

type Datasource struct {
	opts       Options
	prefix     string
	client     *http.Client
	ownsClient bool
}

var _ datasource.Datasource = (*Datasource)(nil)

func New(opts Options) (*Datasource, error) {
	prefix, err := parseBaseURL(opts.URL)
	if err != nil {
		return nil, err
	}
	switch {
	case opts.MaxResponseBytes < 0:
		return nil, datasource.ConfigErrorf("loki: max response bytes must not be negative")
	case opts.MaxResponseBytes > maxConfigurableBytes:
		return nil, datasource.ConfigErrorf("loki: max response bytes must not exceed %d", maxConfigurableBytes)
	case opts.MaxResponseBytes == 0:
		opts.MaxResponseBytes = defaultMaxResponseBytes
	}
	opts.Limits = opts.Limits.WithDefaults(datasource.DefaultLimits)
	if err := opts.Limits.Validate(); err != nil {
		return nil, fmt.Errorf("loki: %w", err)
	}
	if opts.Headers, err = validateHeaders(opts); err != nil {
		return nil, err
	}
	refuseRedirects := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	d := &Datasource{opts: opts, prefix: prefix}
	if opts.HTTPClient == nil {
		d.client = &http.Client{
			Transport:     http.DefaultTransport.(*http.Transport).Clone(),
			CheckRedirect: refuseRedirects,
		}
		d.ownsClient = true
	} else {
		supplied := *opts.HTTPClient
		supplied.CheckRedirect = refuseRedirects
		d.client = &supplied
	}
	return d, nil
}

func validateHeaders(opts Options) (map[string]string, error) {
	if opts.BasicAuthUser == "" && !opts.BasicAuthPassword.IsZero() {
		return nil, datasource.ConfigErrorf("loki: basic auth password requires a user")
	}
	if opts.BasicAuthUser != "" && !opts.BearerToken.IsZero() {
		return nil, datasource.ConfigErrorf("loki: basic auth and bearer token are mutually exclusive")
	}
	headers := make(map[string]string, len(opts.Headers)+1)
	for name, value := range opts.Headers {
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return nil, datasource.ConfigErrorf("loki: invalid header")
		}
		switch {
		case strings.EqualFold(name, "Authorization"):
			return nil, datasource.ConfigErrorf("loki: the Authorization header is set through basic auth or bearer token")
		case strings.EqualFold(name, tenantHeader):
			return nil, datasource.ConfigErrorf("loki: the %s header is set through tenantId", tenantHeader)
		case slices.ContainsFunc(reservedHeaders, func(r string) bool { return strings.EqualFold(name, r) }):
			return nil, datasource.ConfigErrorf("loki: the %s header is managed by the datasource", http.CanonicalHeaderKey(name))
		}
		headers[name] = value
	}
	if opts.TenantID != "" {
		if !httpguts.ValidHeaderFieldValue(opts.TenantID) || strings.TrimSpace(opts.TenantID) != opts.TenantID {
			return nil, datasource.ConfigErrorf("loki: invalid tenant id")
		}
		headers[tenantHeader] = opts.TenantID
	}
	return headers, nil
}

func parseBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", datasource.ConfigErrorf("loki: invalid url")
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", datasource.ConfigErrorf("loki: url scheme must be http or https")
	case u.Hostname() == "":
		return "", datasource.ConfigErrorf("loki: url requires a host")
	case u.User != nil:
		return "", datasource.ConfigErrorf("loki: url must not contain user info")
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", datasource.ConfigErrorf("loki: url must not contain a query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (d *Datasource) Type() string { return "loki" }

func (d *Datasource) Close() error {
	if d.ownsClient {
		d.client.CloseIdleConnections()
	}
	return nil
}

func (d *Datasource) CheckHealth(ctx context.Context) error {
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	return d.ready(ctx)
}
