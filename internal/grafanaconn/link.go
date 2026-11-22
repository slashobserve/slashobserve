package grafanaconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

var (
	ErrNoMatchingConnection = errors.New("no grafana connection matches this link")
	ErrNotDashboardLink     = errors.New("the link is not a grafana dashboard link")
	ErrDashboardNotFound    = errors.New("grafana has no dashboard with this uid that the token can read")
	ErrGrafanaUnavailable   = errors.New("grafana could not return the dashboard")
)

var dashboardUIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

type LinkedDashboard struct {
	ConnectionID   string
	ConnectionName string
	Host           string
	UID            string
	Title          string
	JSON           json.RawMessage
}

func parseWebURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, false
	}
	return u, true
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) && effectivePort(a) == effectivePort(b)
}

func linksTo(link, base string) bool {
	l, ok := parseWebURL(link)
	if !ok {
		return false
	}
	b, ok := parseWebURL(base)
	return ok && sameOrigin(l, b)
}

func dashboardUID(link, base string) (string, bool) {
	l, ok := parseWebURL(link)
	if !ok {
		return "", false
	}
	b, ok := parseWebURL(base)
	if !ok || !sameOrigin(l, b) {
		return "", false
	}
	prefix := strings.TrimRight(b.EscapedPath(), "/")
	rest, found := strings.CutPrefix(l.EscapedPath(), prefix+"/")
	if !found {
		return "", false
	}
	segments := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	if len(segments) < 2 || len(segments) > 3 || (segments[0] != "d" && segments[0] != "d-solo") {
		return "", false
	}
	if !dashboardUIDPattern.MatchString(segments[1]) {
		return "", false
	}
	if len(segments) == 3 && (segments[2] == "" || segments[2] == "." || segments[2] == "..") {
		return "", false
	}
	return segments[1], true
}

func (s *Service) FetchLink(ctx context.Context, link string) (LinkedDashboard, error) {
	if _, ok := parseWebURL(link); !ok {
		return LinkedDashboard{}, ErrNotDashboardLink
	}
	list, err := s.store.list(ctx)
	if err != nil {
		return LinkedDashboard{}, err
	}
	hostMatched := false
	for _, conn := range list {
		if !linksTo(link, conn.URL) {
			continue
		}
		hostMatched = true
		if uid, ok := dashboardUID(link, conn.URL); ok {
			return s.fetchDashboard(ctx, conn, uid)
		}
	}
	if hostMatched {
		return LinkedDashboard{}, ErrNotDashboardLink
	}
	return LinkedDashboard{}, ErrNoMatchingConnection
}

func (s *Service) fetchDashboard(ctx context.Context, conn Connection, uid string) (LinkedDashboard, error) {
	id, err := db.ParseUUID(conn.ID)
	if err != nil {
		return LinkedDashboard{}, err
	}
	_, client, err := s.client(ctx, id)
	if err != nil {
		return LinkedDashboard{}, err
	}
	raw, _, err := client.dashboard(ctx, uid)
	if err != nil {
		return LinkedDashboard{}, s.fetchFailure(ctx, conn.ID, err)
	}
	var envelope struct {
		Dashboard struct {
			Title string `json:"title"`
		} `json:"dashboard"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return LinkedDashboard{}, s.fetchFailure(ctx, conn.ID, errBadResponse)
	}
	return LinkedDashboard{ConnectionID: conn.ID, ConnectionName: conn.Name, Host: hostOf(conn.URL), UID: uid, Title: envelope.Dashboard.Title, JSON: raw}, nil
}

func (s *Service) fetchFailure(ctx context.Context, connectionID string, err error) error {
	var status *egress.StatusError
	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return ErrDashboardNotFound
	}
	if errors.Is(err, egress.ErrNotAllowed) {
		return err
	}
	if !clientgone.Is(ctx, err) {
		s.logger.WarnContext(ctx, "fetch grafana dashboard from a link", logattrs.Failure("grafana_link_fetch_failed", err, slog.String("connection_id", connectionID))...)
	}
	return fmt.Errorf("%w: %w", ErrGrafanaUnavailable, err)
}
