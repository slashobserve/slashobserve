package egress

import (
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

const feedXML = `<rss version="2.0"><channel><title>t</title></channel></rss>`

var rssTypes = []string{"application/rss+xml", "application/atom+xml"}

type fakeAllow struct {
	mu   sync.Mutex
	list domains.List
	err  error
}

func newAllow(t *testing.T, entries ...string) *fakeAllow {
	t.Helper()
	a := &fakeAllow{}
	a.set(t, entries...)
	return a
}

func (a *fakeAllow) set(t *testing.T, entries ...string) {
	t.Helper()
	var list domains.List
	for _, e := range entries {
		p, err := domains.ParsePattern(e)
		if err != nil {
			t.Fatalf("pattern %q: %v", e, err)
		}
		list = append(list, p)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.list = list
}

func (a *fakeAllow) Allowed(context.Context) (domains.List, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.list, a.err
}

type seen struct {
	Path, Cookie, Authorization, UserAgent, Accept, Referer string
}

type upstream struct {
	server *httptest.Server
	hits   atomic.Int64
	mu     sync.Mutex
	seen   []seen
}

func (u *upstream) last() seen {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.seen[len(u.seen)-1]
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "egress test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"feed.test", "other.test", "bad.test", "nope.test", "sub.feed.test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func newUpstream(t *testing.T, allow Allowlist, mux *http.ServeMux) (*upstream, *Client) {
	t.Helper()
	cert, pool := testCertificate(t)
	up := &upstream{}
	up.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up.hits.Add(1)
		up.mu.Lock()
		up.seen = append(up.seen, seen{
			Path: r.URL.Path, Cookie: r.Header.Get("Cookie"), Authorization: r.Header.Get("Authorization"),
			UserAgent: r.Header.Get("User-Agent"), Accept: r.Header.Get("Accept"), Referer: r.Header.Get("Referer"),
		})
		up.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	up.server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	up.server.StartTLS()
	t.Cleanup(up.server.Close)
	addr := up.server.Listener.Addr().String()
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	c := newClient(allow, Options{UserAgent: "TestAgent/1"}, dial, pool)
	t.Cleanup(c.transport.CloseIdleConnections)
	return up, c
}

func serveFeed(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write([]byte(feedXML))
}

func newFeedMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", serveFeed)
	mux.HandleFunc("/cookie", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		serveFeed(w, r)
	})
	mux.HandleFunc("/to-other", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other.test/feed", http.StatusFound)
	})
	mux.HandleFunc("/to-bad", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://bad.test/feed", http.StatusFound)
	})
	mux.HandleFunc("/to-plain", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://other.test/feed", http.StatusFound)
	})
	mux.HandleFunc("/chain/", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/chain/"))
		if n == 0 {
			serveFeed(w, r)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/chain/%d", n-1), http.StatusFound)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	})
	mux.HandleFunc("/none", func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = nil
		_, _ = w.Write([]byte("x"))
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(strings.Repeat("a", 1001)))
	})
	mux.HandleFunc("/exact", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(strings.Repeat("a", 1000)))
	})
	mux.HandleFunc("/declared-big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Length", "5000")
		_, _ = w.Write([]byte(strings.Repeat("a", 5000)))
	})
	mux.HandleFunc("/gzip-big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write([]byte(strings.Repeat("a", 5000)))
		_ = zw.Close()
	})
	return mux
}

func fetchURL(host, path string) string { return "https://" + host + path }

func TestFetchReturnsAllowedFeed(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), newFeedMux())
	before := time.Now()
	resp, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/feed#frag"), Accept: rssTypes})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != feedXML || resp.ContentType != "application/rss+xml" || resp.Charset != "utf-8" {
		t.Errorf("response = %q %q %q", resp.Body, resp.ContentType, resp.Charset)
	}
	if resp.FinalURL != "https://feed.test/feed" {
		t.Errorf("FinalURL = %q", resp.FinalURL)
	}
	if resp.FetchedAt.Before(before) || resp.FetchedAt.After(time.Now()) {
		t.Errorf("FetchedAt = %v", resp.FetchedAt)
	}
	got := up.last()
	if got.UserAgent != "TestAgent/1" || got.Accept != "application/rss+xml, application/atom+xml" {
		t.Errorf("request headers = %+v", got)
	}
}

func TestFetchRefusesUnlistedAndInvalidWithoutNetwork(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		allow    []string
		wantErr  error
		wantHost string
	}{
		{"unlisted host", fetchURL("nope.test", "/feed"), []string{"feed.test"}, ErrNotAllowed, "nope.test"},
		{"empty allowlist", fetchURL("feed.test", "/feed"), nil, ErrNotAllowed, "feed.test"},
		{"apex not matched by wildcard", fetchURL("feed.test", "/feed"), []string{"*.feed.test"}, ErrNotAllowed, "feed.test"},
		{"plain http", "http://feed.test/feed", []string{"feed.test"}, ErrInvalidURL, "feed.test"},
		{"user info", "https://user:pw@feed.test/feed", []string{"feed.test"}, ErrInvalidURL, "feed.test"},
		{"other port", "https://feed.test:8443/feed", []string{"feed.test"}, ErrInvalidURL, "feed.test"},
		{"ipv4 literal", "https://127.0.0.1/feed", []string{"feed.test"}, ErrInvalidURL, "127.0.0.1"},
		{"ipv6 literal", "https://[::1]/feed", []string{"feed.test"}, ErrInvalidURL, "::1"},
		{"single label host", "https://localhost/feed", []string{"feed.test"}, ErrInvalidURL, "localhost"},
		{"no host", "https:///feed", []string{"feed.test"}, ErrInvalidURL, ""},
		{"unparsable", "https://%zz/feed", []string{"feed.test"}, ErrInvalidURL, ""},
		{"no scheme", "feed.test/feed", []string{"feed.test"}, ErrInvalidURL, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up, c := newUpstream(t, newAllow(t, tc.allow...), newFeedMux())
			_, err := c.Fetch(context.Background(), Request{URL: tc.url, Accept: rssTypes})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			var hostErr *HostError
			if tc.wantHost != "" && (!errors.As(err, &hostErr) || hostErr.Host != tc.wantHost) {
				t.Errorf("error = %#v, want HostError for %q", err, tc.wantHost)
			}
			if up.hits.Load() != 0 {
				t.Errorf("upstream saw %d requests", up.hits.Load())
			}
		})
	}
}

func TestFetchFollowsRedirectsOnlyToAllowedHosts(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		allow    []string
		wantErr  error
		wantHost string
	}{
		{"redirect to allowed host", "/to-other", []string{"feed.test", "other.test"}, nil, ""},
		{"redirect to unlisted host", "/to-bad", []string{"feed.test", "other.test"}, ErrNotAllowed, "bad.test"},
		{"redirect to plain http", "/to-plain", []string{"feed.test", "other.test"}, ErrInvalidURL, "other.test"},
		{"five redirects", "/chain/5", []string{"feed.test"}, nil, ""},
		{"six redirects", "/chain/6", []string{"feed.test"}, ErrUnavailable, "feed.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up, c := newUpstream(t, newAllow(t, tc.allow...), newFeedMux())
			resp, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", tc.path), Accept: rssTypes})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil {
				if string(resp.Body) != feedXML {
					t.Errorf("body = %q", resp.Body)
				}
				return
			}
			var hostErr *HostError
			if !errors.As(err, &hostErr) || hostErr.Host != tc.wantHost {
				t.Errorf("error = %#v, want HostError for %q", err, tc.wantHost)
			}
			if errors.Is(tc.wantErr, ErrNotAllowed) && up.hits.Load() != 1 {
				t.Errorf("upstream saw %d requests, want only the first hop", up.hits.Load())
			}
		})
	}
}

func TestRedirectHopsDropReferer(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test", "other.test"), newFeedMux())
	resp, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/to-other"), Accept: rssTypes})
	if err != nil {
		t.Fatal(err)
	}
	if resp.FinalURL != "https://other.test/feed" {
		t.Errorf("FinalURL = %q", resp.FinalURL)
	}
	if got := up.last(); got.Path != "/feed" || got.Referer != "" {
		t.Errorf("redirected request = %+v", got)
	}
}

func TestFetchEnforcesSizeAndContentType(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		accept  []string
		max     int64
		wantErr error
	}{
		{"one byte over the cap", "/big", rssTypes, 1000, ErrTooLarge},
		{"exactly at the cap", "/exact", rssTypes, 1000, nil},
		{"declared length over the cap", "/declared-big", rssTypes, 1000, ErrTooLarge},
		{"gzip body decoded over the cap", "/gzip-big", rssTypes, 1000, ErrTooLarge},
		{"gzip body within the cap", "/gzip-big", rssTypes, 6000, nil},
		{"html when a feed is expected", "/html", rssTypes, 0, ErrContentType},
		{"missing content type", "/none", rssTypes, 0, ErrContentType},
		{"media type matched case-insensitively", "/feed", []string{"Application/RSS+XML"}, 0, nil},
		{"server error", "/boom", rssTypes, 0, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, c := newUpstream(t, newAllow(t, "feed.test"), newFeedMux())
			_, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", tc.path), Accept: tc.accept, MaxBytes: tc.max})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestClampBytes(t *testing.T) {
	cases := map[int64]int64{-5: 1 << 20, 0: 1 << 20, 1: 1, 4096: 4096, 8 << 20: 8 << 20, 9 << 20: 8 << 20}
	for in, want := range cases {
		if got := clampBytes(in); got != want {
			t.Errorf("clampBytes(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestFetchServesRepeatsFromCache(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), newFeedMux())
	req := Request{URL: fetchURL("feed.test", "/feed"), Accept: rssTypes}
	first, err := c.Fetch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Fetch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if up.hits.Load() != 1 {
		t.Errorf("upstream saw %d requests, want 1", up.hits.Load())
	}
	if !first.FetchedAt.Equal(second.FetchedAt) {
		t.Errorf("FetchedAt changed from %v to %v", first.FetchedAt, second.FetchedAt)
	}
	if _, err := c.Fetch(context.Background(), Request{URL: req.URL, Accept: rssTypes, MaxBytes: 5000}); err != nil {
		t.Fatal(err)
	}
	if up.hits.Load() != 2 {
		t.Errorf("a different size cap shared a cache entry: %d requests", up.hits.Load())
	}
}

func TestCachedEntryIsServedOnlyWhileEveryVisitedHostIsAllowed(t *testing.T) {
	cases := []struct {
		name     string
		dropped  []string
		wantHost string
	}{
		{"initial host removed", []string{"other.test"}, "feed.test"},
		{"redirect host removed", []string{"feed.test"}, "other.test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allow := newAllow(t, "feed.test", "other.test")
			up, c := newUpstream(t, allow, newFeedMux())
			req := Request{URL: fetchURL("feed.test", "/to-other"), Accept: rssTypes}
			if _, err := c.Fetch(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			allow.set(t, tc.dropped...)
			_, err := c.Fetch(context.Background(), req)
			var hostErr *HostError
			if !errors.Is(err, ErrNotAllowed) || !errors.As(err, &hostErr) || hostErr.Host != tc.wantHost {
				t.Fatalf("error = %#v, want ErrNotAllowed for %q", err, tc.wantHost)
			}
			if up.hits.Load() != 2 {
				t.Errorf("upstream saw %d requests, want the 2 hops of the first fetch", up.hits.Load())
			}
		})
	}
}

func TestFailuresAreCachedBriefly(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), newFeedMux())
	req := Request{URL: fetchURL("feed.test", "/boom"), Accept: rssTypes}
	for i := 0; i < 2; i++ {
		if _, err := c.Fetch(context.Background(), req); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("fetch %d error = %v", i, err)
		}
	}
	if up.hits.Load() != 1 {
		t.Errorf("upstream saw %d requests, want 1", up.hits.Load())
	}
}

func TestRefusalsAreNotCached(t *testing.T) {
	allow := newAllow(t)
	up, c := newUpstream(t, allow, newFeedMux())
	req := Request{URL: fetchURL("feed.test", "/feed"), Accept: rssTypes}
	if _, err := c.Fetch(context.Background(), req); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("error = %v", err)
	}
	allow.set(t, "feed.test")
	if _, err := c.Fetch(context.Background(), req); err != nil {
		t.Fatalf("after allowing: %v", err)
	}
	if up.hits.Load() != 1 {
		t.Errorf("upstream saw %d requests, want 1", up.hits.Load())
	}
}

func TestBlockedAddressIsReportedAndCached(t *testing.T) {
	allow := newAllow(t, "feed.test")
	var dials atomic.Int64
	dial := func(context.Context, string, string) (net.Conn, error) {
		dials.Add(1)
		return nil, &net.OpError{Op: "dial", Err: refuseBlocked("tcp", "10.0.0.5:443", nil)}
	}
	c := newClient(allow, Options{}, dial, nil)
	req := Request{URL: fetchURL("feed.test", "/feed"), Accept: rssTypes}
	for i := 0; i < 2; i++ {
		_, err := c.Fetch(context.Background(), req)
		var hostErr *HostError
		if !errors.Is(err, ErrBlockedAddress) || !errors.As(err, &hostErr) || hostErr.Host != "feed.test" {
			t.Fatalf("fetch %d error = %#v", i, err)
		}
	}
	if dials.Load() != 1 {
		t.Errorf("dialed %d times, want 1", dials.Load())
	}
}

func TestFetchSendsNoCredentialsOrCookies(t *testing.T) {
	up, c := newUpstream(t, newAllow(t, "feed.test"), newFeedMux())
	if _, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/cookie"), Accept: rssTypes}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/feed"), Accept: rssTypes}); err != nil {
		t.Fatal(err)
	}
	if up.hits.Load() != 2 {
		t.Fatalf("upstream saw %d requests", up.hits.Load())
	}
	got := up.last()
	if got.Cookie != "" || got.Authorization != "" || got.UserAgent != "TestAgent/1" {
		t.Errorf("second request = %+v", got)
	}
}

func TestAllowlistErrorStopsTheFetch(t *testing.T) {
	allow := newAllow(t, "feed.test")
	allow.err = errors.New("store down")
	up, c := newUpstream(t, allow, newFeedMux())
	_, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/feed"), Accept: rssTypes})
	if err == nil || !strings.Contains(err.Error(), "store down") || errors.Is(err, ErrNotAllowed) {
		t.Fatalf("error = %v", err)
	}
	if up.hits.Load() != 0 {
		t.Errorf("upstream saw %d requests", up.hits.Load())
	}
}

func TestErrorTextNeverContainsThePathOrQuery(t *testing.T) {
	const tail = "/secret/path?token=abc"
	cases := []struct {
		name string
		url  string
	}{
		{"unlisted host", fetchURL("nope.test", tail)},
		{"plain http", "http://feed.test" + tail},
		{"other port", "https://feed.test:8443" + tail},
		{"server error", fetchURL("feed.test", "/boom?token=abc")},
		{"wrong content type", fetchURL("feed.test", "/html?token=abc")},
		{"too large", fetchURL("feed.test", "/big?token=abc")},
		{"redirect to unlisted host", fetchURL("feed.test", "/to-bad?token=abc")},
		{"redirect limit", fetchURL("feed.test", "/chain/6?token=abc")},
		{"unparsable", "https://%zz/secret?token=abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, c := newUpstream(t, newAllow(t, "feed.test"), newFeedMux())
			_, err := c.Fetch(context.Background(), Request{URL: tc.url, Accept: rssTypes, MaxBytes: 1000})
			if err == nil {
				t.Fatal("fetch succeeded")
			}
			for _, leak := range []string{"token", "abc", "secret", "/boom", "/html", "/big", "/chain"} {
				if strings.Contains(err.Error(), leak) {
					t.Errorf("error %q contains %q", err, leak)
				}
			}
		})
	}
}

func TestUnreachableHostIsUnavailable(t *testing.T) {
	dial := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("connect: refused for https://feed.test/secret?token=abc")
	}
	c := newClient(newAllow(t, "feed.test"), Options{}, dial, nil)
	_, err := c.Fetch(context.Background(), Request{URL: fetchURL("feed.test", "/secret?token=abc"), Accept: rssTypes})
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "token") {
		t.Fatalf("error = %v", err)
	}
}

func TestHostErrorUnwrapsToItsKind(t *testing.T) {
	err := error(&HostError{Host: "feed.test", Err: ErrNotAllowed})
	if err.Error() != "feed.test: domain not allowed" || !errors.Is(err, ErrNotAllowed) {
		t.Errorf("error = %q", err)
	}
}
