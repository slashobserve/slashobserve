package news

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
	maxURLBytes   = 2000
	maxFeedBytes  = 1 << 20
	feedFetchFail = "news feed fetch failed"
)

var acceptedTypes = []string{
	"application/rss+xml",
	"application/atom+xml",
	"application/rdf+xml",
	"application/xml",
	"text/xml",
}

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

func (h *Handler) Register(api *gin.RouterGroup) {
	api.GET("/news", auth.RequireUser, h.get)
}

type itemJSON struct {
	Title     string  `json:"title"`
	Link      *string `json:"link"`
	Published *string `json:"published"`
	Summary   string  `json:"summary"`
}

type feedJSON struct {
	Title     string     `json:"title"`
	Items     []itemJSON `json:"items"`
	FetchedAt string     `json:"fetched_at"`
}

func (h *Handler) get(c *gin.Context) {
	raw := c.Query("url")
	if raw == "" || len(raw) > maxURLBytes {
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_request", "url is required and must be at most 2000 bytes")
		return
	}
	ctx := c.Request.Context()
	resp, err := h.fetcher.Fetch(ctx, egress.Request{URL: raw, Accept: acceptedTypes, MaxBytes: maxFeedBytes})
	if err != nil {
		h.fetchFailed(c, err)
		return
	}
	base, err := url.Parse(resp.FinalURL)
	if err != nil {
		base = nil
	}
	feed, err := Parse(resp.Body, resp.Charset, base)
	if err != nil {
		host := ""
		if base != nil {
			host = base.Hostname()
		}
		h.refuse(c, http.StatusBadGateway, "feed_invalid", "The address did not return a valid RSS or Atom feed.", host)
		return
	}
	out := feedJSON{Title: feed.Title, Items: make([]itemJSON, 0, len(feed.Items)), FetchedAt: httpapi.FormatTime(resp.FetchedAt)}
	for _, it := range feed.Items {
		item := itemJSON{Title: it.Title, Link: it.Link, Summary: it.Summary}
		if it.Published != nil {
			published := httpapi.FormatTime(*it.Published)
			item.Published = &published
		}
		out.Items = append(out.Items, item)
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) fetchFailed(c *gin.Context, err error) {
	var hostErr *egress.HostError
	host := ""
	if errors.As(err, &hostErr) {
		host = hostErr.Host
	}
	switch {
	case clientgone.Handle(c, h.logger, "fetch news feed", err):
	case errors.Is(err, egress.ErrInvalidURL):
		httpapi.AbortError(c, http.StatusBadRequest, "invalid_feed_url", "Feed URLs must be https addresses with a domain name.")
	case errors.Is(err, egress.ErrNotAllowed):
		h.refuse(c, http.StatusForbidden, "outbound_domain_not_allowed", host+" is not on the outbound allowlist.", host)
	case errors.Is(err, egress.ErrBlockedAddress):
		h.refuse(c, http.StatusBadGateway, "feed_unavailable", host+" points to a private or reserved address.", host)
	case errors.Is(err, egress.ErrTooLarge):
		h.refuse(c, http.StatusBadGateway, "feed_unavailable", "The feed is larger than 1 MiB.", host)
	case errors.Is(err, egress.ErrContentType):
		h.refuse(c, http.StatusBadGateway, "feed_unavailable", "The address did not return an RSS or Atom feed.", host)
	case errors.Is(err, egress.ErrUnavailable):
		h.refuse(c, http.StatusBadGateway, "feed_unavailable", "The feed could not be fetched.", host)
	default:
		h.logger.ErrorContext(c.Request.Context(), "fetch news feed", logattrs.Failure("internal", err, slog.String("actor_id", httpapi.Caller(c).ID))...)
		httpapi.AbortError(c, http.StatusInternalServerError, "internal", "internal error")
	}
}

func (h *Handler) refuse(c *gin.Context, status int, code, message, host string) {
	h.logger.InfoContext(c.Request.Context(), feedFetchFail, slog.String("host", host), slog.String("code", code), slog.String("actor_id", httpapi.Caller(c).ID))
	httpapi.AbortError(c, status, code, message)
}
