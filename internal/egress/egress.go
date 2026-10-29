package egress

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/ttlcache"
)

const (
	defaultCacheTTL = 15 * time.Minute
	failureTTL      = time.Minute
	cacheBytes      = 32 << 20
	defaultMaxBytes = 1 << 20
	hardMaxBytes    = 8 << 20
	maxRedirects    = 5
	fetchTimeout    = 10 * time.Second
	maxConcurrent   = 8
	defaultAgent    = "SlashObserve"
)

var (
	ErrInvalidURL     = errors.New("invalid url")
	ErrNotAllowed     = errors.New("domain not allowed")
	ErrBlockedAddress = errors.New("address not allowed")
	ErrTooLarge       = errors.New("response too large")
	ErrContentType    = errors.New("unexpected content type")
	ErrUnavailable    = errors.New("fetch failed")
)

type Allowlist interface {
	Allowed(ctx context.Context) (domains.List, error)
}

type Request struct {
	URL      string
	Accept   []string
	MaxBytes int64
}

type Response struct {
	Body        []byte
	ContentType string
	Charset     string
	FinalURL    string
	FetchedAt   time.Time
}

type Options struct {
	UserAgent string
	CacheTTL  time.Duration
}

type HostError struct {
	Host string
	Err  error
}

func (e *HostError) Error() string { return e.Host + ": " + e.Err.Error() }

func (e *HostError) Unwrap() error { return e.Err }

type Client struct {
	allow     Allowlist
	userAgent string
	transport *http.Transport
	hits      *ttlcache.Cache[string, visited]
	failures  *ttlcache.Cache[string, error]
	group     *ttlcache.Group[visited]
	slots     chan struct{}
}

type visited struct {
	resp  Response
	hosts []string
}

type job struct {
	key      string
	url      *url.URL
	host     string
	accept   []string
	maxBytes int64
	list     domains.List
}

func New(allow Allowlist, opts Options) *Client {
	return newClient(allow, opts, nil, nil)
}

func newClient(allow Allowlist, opts Options, dial dialFunc, roots *x509.CertPool) *Client {
	if opts.UserAgent == "" {
		opts.UserAgent = defaultAgent
	}
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = defaultCacheTTL
	}
	return &Client{
		allow:     allow,
		userAgent: opts.UserAgent,
		transport: newTransport(dial, roots),
		hits:      ttlcache.New[string, visited](opts.CacheTTL, cacheBytes),
		failures:  ttlcache.New[string, error](failureTTL, cacheBytes),
		group:     ttlcache.NewGroup[visited](fetchTimeout),
		slots:     make(chan struct{}, maxConcurrent),
	}
}

func (c *Client) Fetch(ctx context.Context, req Request) (Response, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return Response{}, ErrInvalidURL
	}
	u.Fragment, u.RawFragment = "", ""
	host, err := hostOf(u)
	if err != nil {
		return Response{}, err
	}
	list, err := c.allow.Allowed(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("read outbound allowlist: %w", err)
	}
	if !list.Allows(host) {
		return Response{}, &HostError{Host: host, Err: ErrNotAllowed}
	}
	j := job{url: u, host: host, accept: req.Accept, maxBytes: clampBytes(req.MaxBytes), list: list}
	j.key = u.String() + "\x00" + strings.Join(req.Accept, ",") + "\x00" + strconv.FormatInt(j.maxBytes, 10)

	if hit, at, ok := c.hits.Get(j.key); ok {
		for _, h := range hit.hosts {
			if !list.Allows(h) {
				return Response{}, &HostError{Host: h, Err: ErrNotAllowed}
			}
		}
		hit.resp.FetchedAt = at
		return hit.resp, nil
	}
	if cached, _, ok := c.failures.Get(j.key); ok {
		return Response{}, cached
	}
	got, err := c.group.Do(ctx, j.key, func(ctx context.Context) (visited, error) { return c.run(ctx, j) })
	if err != nil {
		return Response{}, err
	}
	return got.resp, nil
}

func clampBytes(n int64) int64 {
	switch {
	case n <= 0:
		return defaultMaxBytes
	case n > hardMaxBytes:
		return hardMaxBytes
	}
	return n
}

func invalid(host string) error {
	if host == "" {
		return ErrInvalidURL
	}
	return &HostError{Host: host, Err: ErrInvalidURL}
}

func hostOf(u *url.URL) (string, error) {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return "", invalid(u.Hostname())
	}
	host, err := domains.NormalizeHost(u.Hostname())
	if err != nil {
		return "", invalid(u.Hostname())
	}
	return host, nil
}

func (c *Client) run(ctx context.Context, j job) (visited, error) {
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return visited{}, ctx.Err()
	}
	got, err := c.download(ctx, j)
	if err == nil {
		got.resp.FetchedAt = c.hits.Put(j.key, got, int64(len(got.resp.Body)))
		return got, nil
	}
	if cacheable(err) {
		c.failures.Put(j.key, err, 1)
	}
	return visited{}, err
}

func cacheable(err error) bool {
	return errors.Is(err, ErrBlockedAddress) || errors.Is(err, ErrTooLarge) ||
		errors.Is(err, ErrContentType) || errors.Is(err, ErrUnavailable)
}

type trail struct {
	list  domains.List
	hosts []string
}

func (t *trail) last() string { return t.hosts[len(t.hosts)-1] }

func (t *trail) visit(u *url.URL) error {
	host, err := hostOf(u)
	if err != nil {
		return err
	}
	if !t.list.Allows(host) {
		return &HostError{Host: host, Err: ErrNotAllowed}
	}
	t.hosts = append(t.hosts, host)
	return nil
}

func (t *trail) fail(err error) error {
	var hostErr *HostError
	switch {
	case errors.As(err, &hostErr):
		return hostErr
	case errors.Is(err, ErrBlockedAddress):
		return &HostError{Host: t.last(), Err: ErrBlockedAddress}
	case errors.Is(err, context.Canceled):
		return context.Canceled
	}
	return &HostError{Host: t.last(), Err: ErrUnavailable}
}

func (c *Client) download(ctx context.Context, j job) (visited, error) {
	t := &trail{list: j.list, hosts: []string{j.host}}
	client := &http.Client{
		Transport: c.transport,
		Timeout:   fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return &HostError{Host: t.last(), Err: ErrUnavailable}
			}
			req.Header.Del("Referer")
			return t.visit(req.URL)
		},
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url.String(), nil)
	if err != nil {
		return visited{}, t.fail(err)
	}
	hreq.Header.Set("User-Agent", c.userAgent)
	if len(j.accept) > 0 {
		hreq.Header.Set("Accept", strings.Join(j.accept, ", "))
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return visited{}, t.fail(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return readBody(resp, j, t)
}

func readBody(resp *http.Response, j job, t *trail) (visited, error) {
	fail := func(kind error) (visited, error) { return visited{}, &HostError{Host: t.last(), Err: kind} }
	if resp.StatusCode != http.StatusOK {
		return fail(ErrUnavailable)
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !slices.ContainsFunc(j.accept, func(a string) bool { return strings.EqualFold(a, mediaType) }) {
		return fail(ErrContentType)
	}
	if resp.ContentLength > j.maxBytes {
		return fail(ErrTooLarge)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, j.maxBytes+1))
	if err != nil {
		return visited{}, t.fail(err)
	}
	if int64(len(body)) > j.maxBytes {
		return fail(ErrTooLarge)
	}
	return visited{
		resp: Response{
			Body:        body,
			ContentType: mediaType,
			Charset:     params["charset"],
			FinalURL:    resp.Request.URL.String(),
		},
		hosts: t.hosts,
	}, nil
}
