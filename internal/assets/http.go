package assets

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	maxURLBytes     = 2000
	maxAssetBytes   = 5 << 20
	assetFetchFail  = "asset fetch failed"
	cacheControl    = "private, max-age=3600"
	imageSandboxCSP = "default-src 'none'; style-src 'unsafe-inline'; sandbox; frame-ancestors 'none'"
)

var (
	imageTypes = []string{
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/webp",
		"image/avif",
		"image/svg+xml",
	}
	geojsonAccepted = []string{
		"application/geo+json",
		"application/json",
		"text/plain",
	}
)

type Fetcher interface {
	Fetch(context.Context, egress.Request) (egress.Response, error)
}

type Handler struct {
	fetcher Fetcher
	logger  *slog.Logger
}

func NewHandler(f Fetcher, logger *slog.Logger) *Handler {
	return &Handler{fetcher: f, logger: logger}
}

func (h *Handler) Register(api auth.Routes) {
	api.GET("/assets/image", auth.RoleViewer, auth.RequireUser, h.image)
	api.GET("/assets/geojson", auth.RoleViewer, auth.RequireUser, h.geojson)
}

func (h *Handler) image(c *gin.Context) {
	resp, ok := h.fetch(c, imageTypes, "an image")
	if !ok {
		return
	}
	if !sniffImage(resp.Body, resp.ContentType) {
		h.refuse(c, http.StatusBadGateway, "asset_unavailable", "The address did not return a valid image.", finalHost(resp))
		return
	}
	c.Header("Cache-Control", cacheControl)
	c.Header("Content-Security-Policy", imageSandboxCSP)
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Data(http.StatusOK, resp.ContentType, resp.Body)
}

func (h *Handler) geojson(c *gin.Context) {
	resp, ok := h.fetch(c, geojsonAccepted, "a GeoJSON file")
	if !ok {
		return
	}
	body, valid := validGeoJSON(resp.Body)
	if !valid {
		h.refuse(c, http.StatusBadGateway, "asset_unavailable", "The address did not return a valid GeoJSON file.", finalHost(resp))
		return
	}
	c.Header("Cache-Control", cacheControl)
	c.Data(http.StatusOK, "application/json", body)
}

func (h *Handler) fetch(c *gin.Context, accept []string, noun string) (egress.Response, bool) {
	raw := c.Query("url")
	if raw == "" || len(raw) > maxURLBytes {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "url is required and must be at most 2000 bytes")
		return egress.Response{}, false
	}
	resp, err := h.fetcher.Fetch(c.Request.Context(), egress.Request{URL: raw, Accept: accept, MaxBytes: maxAssetBytes})
	if err != nil {
		h.fetchFailed(c, err, noun)
		return egress.Response{}, false
	}
	return resp, true
}

func (h *Handler) fetchFailed(c *gin.Context, err error, noun string) {
	var hostErr *egress.HostError
	host := ""
	if errors.As(err, &hostErr) {
		host = hostErr.Host
	}
	switch {
	case clientgone.Handle(c, h.logger, "fetch asset", err):
	case errors.Is(err, egress.ErrInvalidURL):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_asset_url", "Asset URLs must be https addresses with a domain name.")
	case errors.Is(err, egress.ErrNotAllowed):
		h.refuse(c, http.StatusForbidden, "outbound_domain_not_allowed", host+" is not on the outbound allowlist.", host)
	case errors.Is(err, egress.ErrBlockedAddress):
		h.refuse(c, http.StatusBadGateway, "asset_unavailable", host+" points to a private or reserved address.", host)
	case errors.Is(err, egress.ErrTooLarge):
		h.refuse(c, http.StatusBadGateway, "asset_unavailable", "The file is larger than 5 MiB.", host)
	case errors.Is(err, egress.ErrContentType):
		h.refuse(c, http.StatusBadGateway, "asset_unavailable", "The address did not return "+noun+".", host)
	case errors.Is(err, egress.ErrUnavailable):
		h.refuse(c, http.StatusBadGateway, "asset_unavailable", "The file could not be fetched.", host)
	default:
		h.logger.ErrorContext(c.Request.Context(), "fetch asset", logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

func (h *Handler) refuse(c *gin.Context, status int, code, message, host string) {
	h.logger.InfoContext(c.Request.Context(), assetFetchFail, slog.String("host", host), slog.String("code", code), slog.String("actor_id", httpapi.Caller(c).ID))
	httpapi.AbortError(c, status, code, message)
}

func finalHost(resp egress.Response) string {
	u, err := url.Parse(resp.FinalURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
