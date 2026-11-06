package elasticsearch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/http/httpguts"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

type Flavor string

const (
	Elasticsearch Flavor = "elasticsearch"
	OpenSearch    Flavor = "opensearch"
)

const (
	QueryLanguageKQL    = "kql"
	QueryLanguageLucene = "lucene"

	DefaultTimeField = "@timestamp"

	defaultMaxResponseBytes = 64 << 20
	maxConfigurableBytes    = 1 << 30
)

var (
	queryLanguages  = []string{QueryLanguageKQL, QueryLanguageLucene}
	reservedHeaders = []string{"Accept", "Host", "Content-Length", "Content-Type"}
)

type FieldRoles struct {
	Time, Message, Level, Service, TraceID, SpanID string
}

type Options struct {
	Flavor            Flavor
	URL               string
	Headers           map[string]string
	BasicAuthUser     string
	BasicAuthPassword datasource.Secret
	BearerToken       datasource.Secret
	APIKey            datasource.Secret
	Index             string
	TimeField         string
	QueryLanguage     string
	Fields            FieldRoles
	Limits            datasource.Limits
	HTTPClient        *http.Client
	MaxResponseBytes  int64
}

type Datasource struct {
	opts       Options
	prefix     string
	client     *http.Client
	ownsClient bool
	mapping    mappingCache
}

var _ datasource.Datasource = (*Datasource)(nil)

func New(opts Options) (*Datasource, error) {
	if opts.Flavor != Elasticsearch && opts.Flavor != OpenSearch {
		return nil, datasource.ConfigErrorf("search datasource: flavor must be %s or %s", Elasticsearch, OpenSearch)
	}
	name := string(opts.Flavor)
	prefix, err := parseBaseURL(name, opts.URL)
	if err != nil {
		return nil, err
	}
	switch {
	case opts.Index == "":
		return nil, datasource.ConfigErrorf("%s: index is required", name)
	case strings.ContainsFunc(opts.Index, unicode.IsSpace):
		return nil, datasource.ConfigErrorf("%s: index must not contain whitespace", name)
	}
	if opts.TimeField == "" {
		opts.TimeField = DefaultTimeField
	}
	opts.Fields.Time = opts.TimeField
	if opts.QueryLanguage == "" {
		opts.QueryLanguage = QueryLanguageKQL
	}
	if !slices.Contains(queryLanguages, opts.QueryLanguage) {
		return nil, datasource.ConfigErrorf("%s: query language must be %s", name, strings.Join(queryLanguages, " or "))
	}
	switch {
	case opts.MaxResponseBytes < 0:
		return nil, datasource.ConfigErrorf("%s: max response bytes must not be negative", name)
	case opts.MaxResponseBytes > maxConfigurableBytes:
		return nil, datasource.ConfigErrorf("%s: max response bytes must not exceed %d", name, maxConfigurableBytes)
	case opts.MaxResponseBytes == 0:
		opts.MaxResponseBytes = defaultMaxResponseBytes
	}
	opts.Limits = opts.Limits.WithDefaults(datasource.DefaultLimits)
	if err := opts.Limits.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if opts.Headers, err = validateHeaders(name, opts); err != nil {
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

func validateHeaders(name string, opts Options) (map[string]string, error) {
	credentials := 0
	for _, set := range []bool{opts.BasicAuthUser != "", !opts.BearerToken.IsZero(), !opts.APIKey.IsZero()} {
		if set {
			credentials++
		}
	}
	switch {
	case opts.BasicAuthUser == "" && !opts.BasicAuthPassword.IsZero():
		return nil, datasource.ConfigErrorf("%s: basic auth password requires a user", name)
	case credentials > 1:
		return nil, datasource.ConfigErrorf("%s: basic auth, bearer token and api key are mutually exclusive", name)
	}
	headers := make(map[string]string, len(opts.Headers))
	for header, value := range opts.Headers {
		if !httpguts.ValidHeaderFieldName(header) || !httpguts.ValidHeaderFieldValue(value) {
			return nil, datasource.ConfigErrorf("%s: invalid header", name)
		}
		switch {
		case strings.EqualFold(header, "Authorization"):
			return nil, datasource.ConfigErrorf("%s: the Authorization header is set through basic auth, bearer token or api key", name)
		case slices.ContainsFunc(reservedHeaders, func(r string) bool { return strings.EqualFold(header, r) }):
			return nil, datasource.ConfigErrorf("%s: the %s header is managed by the datasource", name, http.CanonicalHeaderKey(header))
		}
		headers[header] = value
	}
	return headers, nil
}

func parseBaseURL(name, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", datasource.ConfigErrorf("%s: invalid url", name)
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return "", datasource.ConfigErrorf("%s: url scheme must be http or https", name)
	case u.Hostname() == "":
		return "", datasource.ConfigErrorf("%s: url requires a host", name)
	case u.User != nil:
		return "", datasource.ConfigErrorf("%s: url must not contain user info", name)
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
		return "", datasource.ConfigErrorf("%s: url must not contain a query or fragment", name)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (d *Datasource) Type() string { return string(d.opts.Flavor) }

func (d *Datasource) Close() error {
	if d.ownsClient {
		d.client.CloseIdleConnections()
	}
	return nil
}

func (d *Datasource) dialect() macros.Dialect {
	if d.opts.Flavor == OpenSearch {
		return macros.OpenSearch
	}
	return macros.Elasticsearch
}

func (d *Datasource) expand(q datasource.Query, r datasource.TimeRange, interval time.Duration) (string, error) {
	exp, err := macros.Expand(d.dialect(), q.Text, macros.Env{
		Range:     r,
		Interval:  interval,
		Variables: q.Variables,
		Dashboard: q.Dashboard,
		User:      q.User,
	})
	if err != nil {
		return "", err
	}
	return exp.Text, nil
}

func instantInterval(r datasource.TimeRange, minimum time.Duration) time.Duration {
	return max(time.Second, (r.To.Sub(r.From) / 1000).Truncate(time.Second), minimum)
}

func (d *Datasource) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}

func (d *Datasource) QueryRange(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
	return nil, d.queriesUnavailable()
}

func (d *Datasource) QueryInstant(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
	return nil, d.queriesUnavailable()
}

func (d *Datasource) queriesUnavailable() error {
	return datasource.Errorf(datasource.ErrInvalidQuery, "queries on %s are not available yet; field discovery and variables are", d.opts.Flavor)
}
