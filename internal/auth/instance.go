package auth

import (
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

type instanceResponse struct {
	PublicURL         string   `json:"public_url"`
	MCPURL            string   `json:"mcp_url"`
	OIDCIssuerURL     string   `json:"oidc_issuer_url"`
	OIDCProviderName  string   `json:"oidc_provider_name"`
	AllowedDomains    []string `json:"allowed_email_domains"`
	AnyEmailDomain    bool     `json:"any_email_domain"`
	SessionTTLSeconds int64    `json:"session_ttl_seconds"`
}

func (s *Service) instance(c *gin.Context) {
	domains := []string{}
	if !s.cfg.AllowAnyEmailDomain {
		domains = append(domains, s.cfg.AllowedEmailDomains...)
	}
	c.JSON(http.StatusOK, instanceResponse{
		PublicURL:         s.cfg.PublicURL.String(),
		MCPURL:            s.cfg.PublicURL.JoinPath("mcp").String(),
		OIDCIssuerURL:     s.issuerWithoutUserInfo(),
		OIDCProviderName:  s.cfg.ProviderName,
		AllowedDomains:    domains,
		AnyEmailDomain:    s.cfg.AllowAnyEmailDomain,
		SessionTTLSeconds: int64(s.cfg.SessionTTL / time.Second),
	})
}

func (s *Service) issuerWithoutUserInfo() string {
	u, err := url.Parse(s.cfg.IssuerURL)
	if err != nil {
		return ""
	}
	u.User = nil

	return u.String()
}
