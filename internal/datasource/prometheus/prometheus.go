package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	defaultScrapeInterval   = 15 * time.Second
	defaultMaxResponseBytes = 64 << 20
	maxConfigurableBytes    = 1 << 30
)

var reservedHeaders = []string{"Content-Type", "Accept", "Host", "Content-Length"}

type Options struct {
	URL               string
	ScrapeInterval    time.Duration
	Headers           map[string]string
	BasicAuthUser     string
	BasicAuthPassword datasource.Secret
	BearerToken       datasource.Secret
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
	if opts.ScrapeInterval < 0 {
		return nil, datasource.ConfigErrorf("prometheus: scrape interval must not be negative")
	}
	if opts.MaxResponseBytes < 0 {
		return nil, datasource.ConfigErrorf("prometheus: max response bytes must not be negative")
	}
	if opts.MaxResponseBytes > maxConfigurableBytes {
		return nil, datasource.ConfigErrorf("prometheus: max response bytes must not exceed %d", maxConfigurableBytes)
	}
	if opts.ScrapeInterval == 0 {
		opts.ScrapeInterval = defaultScrapeInterval
	}
	if opts.MaxResponseBytes == 0 {
		opts.MaxResponseBytes = defaultMaxResponseBytes
	}
	opts.Limits = opts.Limits.WithDefaults(datasource.DefaultLimits)
	if err := opts.Limits.Validate(); err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}
	if opts.Headers, err = validateAuth(opts); err != nil {
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

func validateAuth(opts Options) (map[string]string, error) {
	if opts.BasicAuthUser == "" && !opts.BasicAuthPassword.IsZero() {
		return nil, datasource.ConfigErrorf("prometheus: basic auth password requires a user")
	}
	if opts.BasicAuthUser != "" && !opts.BearerToken.IsZero() {
		return nil, datasource.ConfigErrorf("prometheus: basic auth and bearer token are mutually exclusive")
	}
	headers := make(map[string]string, len(opts.Headers))
	for name, value := range opts.Headers {
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return nil, datasource.ConfigErrorf("prometheus: invalid header")
		}
		if strings.EqualFold(name, "Authorization") {
			return nil, datasource.ConfigErrorf("prometheus: the Authorization header is set through basic auth or bearer token")
		}
		if slices.ContainsFunc(reservedHeaders, func(r string) bool { return strings.EqualFold(name, r) }) {
			return nil, datasource.ConfigErrorf("prometheus: the %s header is managed by the datasource", http.CanonicalHeaderKey(name))
		}
		headers[name] = value
	}
	return headers, nil
}

func parseBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", datasource.ConfigErrorf("prometheus: invalid url")
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", datasource.ConfigErrorf("prometheus: url scheme must be http or https")
	case u.Hostname() == "":
		return "", datasource.ConfigErrorf("prometheus: url requires a host")
	case u.User != nil:
		return "", datasource.ConfigErrorf("prometheus: url must not contain user info")
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", datasource.ConfigErrorf("prometheus: url must not contain a query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (d *Datasource) Type() string { return "prometheus" }

func (d *Datasource) Close() error {
	if d.ownsClient {
		d.client.CloseIdleConnections()
	}
	return nil
}

func (d *Datasource) CheckHealth(ctx context.Context) error {
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	_, err := d.do(ctx, http.MethodPost, "/api/v1/query", url.Values{"query": {"1"}})
	return err
}
