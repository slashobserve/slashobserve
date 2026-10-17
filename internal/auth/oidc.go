package auth

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const backChannelTimeout = 10 * time.Second

type issuerRewrite struct {
	from, to *url.URL
	base     http.RoundTripper
}

func (t issuerRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == t.from.Scheme && r.URL.Host == t.from.Host {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host, r.Host = t.to.Scheme, t.to.Host, ""
	}
	return t.base.RoundTrip(r)
}

func newBackChannelClient(issuer, internal *url.URL) *http.Client {
	client := &http.Client{Timeout: backChannelTimeout}
	if internal != nil {
		client.Transport = issuerRewrite{from: issuer, to: internal, base: http.DefaultTransport}
	}
	return client
}

func (s *Service) backChannel(ctx context.Context) context.Context {
	return oidc.ClientContext(ctx, s.client)
}

func (s *Service) provider(ctx context.Context) (*oidc.Provider, error) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.oidcProvider != nil {
		return s.oidcProvider, nil
	}
	p, err := oidc.NewProvider(s.backChannel(ctx), s.cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	s.oidcProvider = p
	return p, nil
}

func (s *Service) oauthConfig(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.cfg.PublicURL.String() + "/api/v1/auth/callback",
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}
