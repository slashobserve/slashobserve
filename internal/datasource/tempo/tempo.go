package tempo

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
		return nil, datasource.ConfigErrorf("tempo: max response bytes must not be negative")
	case opts.MaxResponseBytes > maxConfigurableBytes:
		return nil, datasource.ConfigErrorf("tempo: max response bytes must not exceed %d", maxConfigurableBytes)
	case opts.MaxResponseBytes == 0:
		opts.MaxResponseBytes = defaultMaxResponseBytes
	}
	opts.Limits = opts.Limits.WithDefaults(datasource.DefaultLimits)
	if err := opts.Limits.Validate(); err != nil {
		return nil, fmt.Errorf("tempo: %w", err)
	}
	if err := validateHeaders(opts); err != nil {
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

func validateHeaders(opts Options) error {
	if opts.BasicAuthUser == "" && !opts.BasicAuthPassword.IsZero() {
		return datasource.ConfigErrorf("tempo: basic auth password requires a user")
	}
	if opts.BasicAuthUser != "" && !opts.BearerToken.IsZero() {
		return datasource.ConfigErrorf("tempo: basic auth and bearer token are mutually exclusive")
	}
	for name, value := range opts.Headers {
		if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
			return datasource.ConfigErrorf("tempo: invalid header")
		}
		switch {
		case strings.EqualFold(name, "Authorization"):
			return datasource.ConfigErrorf("tempo: the Authorization header is set through basic auth or bearer token")
		case slices.ContainsFunc(reservedHeaders, func(r string) bool { return strings.EqualFold(name, r) }):
			return datasource.ConfigErrorf("tempo: the %s header is managed by the datasource", http.CanonicalHeaderKey(name))
		}
	}
	return nil
}

func parseBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", datasource.ConfigErrorf("tempo: invalid url")
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", datasource.ConfigErrorf("tempo: url scheme must be http or https")
	case u.Hostname() == "":
		return "", datasource.ConfigErrorf("tempo: url requires a host")
	case u.User != nil:
		return "", datasource.ConfigErrorf("tempo: url must not contain user info")
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", datasource.ConfigErrorf("tempo: url must not contain a query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (d *Datasource) Type() string { return "tempo" }

func (d *Datasource) Close() error {
	if d.ownsClient {
		d.client.CloseIdleConnections()
	}
	return nil
}

func (d *Datasource) CheckHealth(ctx context.Context) error {
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	_, err := d.get(ctx, "/ready", nil)
	return err
}
