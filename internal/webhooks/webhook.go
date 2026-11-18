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

	maxWaitingCount = 100
)

const (
	FormatGeneric = "generic"
	FormatSlack   = "slack"
	FormatTeams   = "teams"
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
	ErrNothingToSkip    = errors.New("the webhook is not holding back an event")
	ErrBusy             = errors.New("the webhook is sending right now; try again in a few seconds")
)

type InvalidError struct {
	Message string
}

func (e *InvalidError) Error() string { return e.Message }

type Webhook struct {
	ID                  string
	Name                string
	URL                 string
	Format              string
	EventTypes          []string
	Owner               actor.Actor
	Enabled             bool
	DisabledReason      string
	DisabledAt          *time.Time
	ConsecutiveFailures int
	NextAttemptAt       time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Waiting             *Waiting
	at                  cursor
}

type Waiting struct {
	Count int
	More  bool
}

func webhookOf(row dbsqlc.GetWebhookRow) Webhook {
	w := Webhook{
		ID:         row.ID.String(),
		Name:       row.Name,
		URL:        row.Url,
		Format:     row.Format,
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
		at:                  cursor{xid: row.CursorXid.Uint64, seq: row.CursorSeq},
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
	text := strings.TrimSpace(raw)
	switch {
	case text == "":
		return "", "", urlProblem("url is required")
	case len(text) > maxURLBytes:
		return "", "", urlProblem("url can be at most %d characters", maxURLBytes)
	}
	u, err := url.Parse(text)
	switch {
	case err != nil:
		return "", "", urlProblem("url is not a valid address; write it like https://hooks.example.com/slashobserve")
	case u.Scheme != "https":
		return "", "", urlProblem("url must start with https://, got %q", u.Scheme)
	case u.Opaque != "" || u.Host == "":
		return "", "", urlProblem("url must be a full address with a domain after https://")
	case u.User != nil:
		return "", "", urlProblem("url must not contain a user name or password")
	case u.Fragment != "" || strings.Contains(text, "#"):
		return "", "", urlProblem("url must not contain a # fragment")
	case u.Port() != "" && u.Port() != "443":
		return "", "", urlProblem("url must use port 443, got %s", u.Port())
	}
	host, err = domains.NormalizeHost(u.Hostname())
	if err != nil {
		return "", "", urlProblem("url host: %s", err.Error())
	}
	u.Host = host
	return u.String(), host, nil
}

func urlProblem(format string, args ...any) error {
	return &InvalidError{Message: fmt.Sprintf(format, args...)}
}

func cleanFormat(raw string) (string, error) {
	switch raw {
	case FormatGeneric, FormatSlack, FormatTeams:
		return raw, nil
	}
	return "", &InvalidError{Message: fmt.Sprintf("format must be %s, %s or %s", FormatGeneric, FormatSlack, FormatTeams)}
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
