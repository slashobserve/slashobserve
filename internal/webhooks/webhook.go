package webhooks

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
)

const (
	maxWebhooks  = 20
	maxNameRunes = 100
	maxURLBytes  = 2048
)

const (
	ReasonManual           = "manual"
	ReasonGone             = "gone"
	ReasonFailing          = "failing"
	ReasonOwner            = "owner"
	ReasonSecretUnreadable = "secret_unreadable"
)

var (
	ErrNotFound         = errors.New("webhook not found")
	ErrTooMany          = errors.New("too many webhooks")
	ErrNoSecretKey      = errors.New("SLASHOBSERVE_SECRET_KEY must be set to use webhooks")
	ErrSecretUnreadable = errors.New("the signing secret cannot be read with the current SLASHOBSERVE_SECRET_KEY; rotate the webhook's secret")
	ErrInvalidCursor    = errors.New("invalid cursor")
)

type InvalidError struct {
	Message string
}

func (e *InvalidError) Error() string { return e.Message }

type Webhook struct {
	ID                  string
	Name                string
	URL                 string
	EventTypes          []string
	Owner               actor.Actor
	Enabled             bool
	DisabledReason      string
	DisabledAt          *time.Time
	ConsecutiveFailures int
	NextAttemptAt       time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func webhookOf(row dbsqlc.GetWebhookRow) Webhook {
	w := Webhook{
		ID:         row.ID.String(),
		Name:       row.Name,
		URL:        row.Url,
		EventTypes: row.EventTypes,
		Owner: actor.FromColumns(actor.Columns{
			Kind: row.OwnerKind,
			ID:   row.OwnerID,
			Name: row.OwnerName,
		}),
		Enabled:             !row.DisabledAt.Valid,
		ConsecutiveFailures: int(row.ConsecutiveFailures),
		NextAttemptAt:       row.NextAttemptAt.Time.UTC(),
		CreatedAt:           row.CreatedAt.Time.UTC(),
		UpdatedAt:           row.UpdatedAt.Time.UTC(),
	}
	if row.DisabledAt.Valid {
		at := row.DisabledAt.Time.UTC()
		w.DisabledAt = &at
		w.DisabledReason = row.DisabledReason.String
	}
	return w
}

func cleanName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes || strings.ContainsFunc(name, unicode.IsControl) {
		return "", &InvalidError{Message: fmt.Sprintf("name must be 1 to %d characters without control characters", maxNameRunes)}
	}
	return name, nil
}

func cleanURL(raw string) (address, host string, err error) {
	invalid := &InvalidError{Message: "url must be an https address on port 443 with a domain name, no user info and no fragment"}
	text := strings.TrimSpace(raw)
	if text == "" || len(text) > maxURLBytes {
		return "", "", invalid
	}
	u, err := url.Parse(text)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" || (u.Port() != "" && u.Port() != "443") {
		return "", "", invalid
	}
	host, err = domains.NormalizeHost(u.Hostname())
	if err != nil {
		return "", "", invalid
	}
	u.Host = host
	return u.String(), host, nil
}

func cleanEventTypes(types []string) ([]string, error) {
	known := len(activity.Types())
	if len(types) == 0 || len(types) > known {
		return nil, &InvalidError{Message: fmt.Sprintf("event_types must hold 1 to %d activity types", known)}
	}
	seen := make(map[string]bool, len(types))
	for _, t := range types {
		if !activity.Known(t) {
			return nil, &InvalidError{Message: fmt.Sprintf("%q is not an activity type", t)}
		}
		if seen[t] {
			return nil, &InvalidError{Message: fmt.Sprintf("event type %q is listed twice", t)}
		}
		seen[t] = true
	}
	return slices.Clone(types), nil
}
