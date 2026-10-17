package prometheus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	defaultScrapeInterval   = 15 * time.Second
	defaultMaxResponseBytes = 64 << 20
)

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
	opts   Options
	prefix string
	client *http.Client
}

var _ datasource.Datasource = (*Datasource)(nil)

func New(opts Options) (*Datasource, error) {
	prefix, err := parseBaseURL(opts.URL)
	if err != nil {
		return nil, err
	}
	if opts.ScrapeInterval < 0 {
		return nil, errors.New("prometheus: scrape interval must not be negative")
	}
	if opts.MaxResponseBytes < 0 {
		return nil, errors.New("prometheus: max response bytes must not be negative")
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
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Transport: http.DefaultTransport.(*http.Transport).Clone(),
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Datasource{opts: opts, prefix: prefix, client: client}, nil
}

func validateAuth(opts Options) (map[string]string, error) {
	if opts.BasicAuthUser == "" && !opts.BasicAuthPassword.IsZero() {
		return nil, errors.New("prometheus: basic auth password requires a user")
	}
	if opts.BasicAuthUser != "" && !opts.BearerToken.IsZero() {
		return nil, errors.New("prometheus: basic auth and bearer token are mutually exclusive")
	}
	headers := make(map[string]string, len(opts.Headers))
	for name, value := range opts.Headers {
		if name == "" || strings.ContainsAny(name, ": \t\r\n") || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("prometheus: invalid header")
		}
		if strings.EqualFold(name, "Authorization") {
			return nil, errors.New("prometheus: the Authorization header is set through basic auth or bearer token")
		}
		headers[name] = value
	}
	return headers, nil
}

func parseBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("prometheus: invalid url")
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", errors.New("prometheus: url scheme must be http or https")
	case u.Hostname() == "":
		return "", errors.New("prometheus: url requires a host")
	case u.User != nil:
		return "", errors.New("prometheus: url must not contain user info")
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", errors.New("prometheus: url must not contain a query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (d *Datasource) Type() string { return "prometheus" }

func (d *Datasource) Close() error {
	d.client.CloseIdleConnections()
	return nil
}

func (d *Datasource) CheckHealth(ctx context.Context) error {
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	_, err := d.do(ctx, http.MethodPost, "/api/v1/query", url.Values{"query": {"1"}})
	return err
}
