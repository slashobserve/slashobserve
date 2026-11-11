package claims

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	tokenPrefix          = "sot_"
	maxWait              = 30 * time.Second
	pollEvery            = 5 * time.Second
	maxWaitersPerClaimer = 2
	maxWaiters           = 64
	retryAfter           = 5 * time.Second
	touchEvery           = 15 * time.Second
	maxClientRunes       = 200
	maxNoteRunes         = 500
	maxMessageRunes      = 500
	maxUsageTextRunes    = 100
	maxCostUSD           = 10000.0
	defaultRunTime       = time.Hour
)

var (
	ErrNotAWorker = errors.New("this credential does not take runs")
	ErrNotYourRun = errors.New("this run is not claimed by you at that attempt")
)

type Config struct {
	DB         *db.DB
	Runs       *revisions.Service
	Listen     events.Listener
	PublicURL  *url.URL
	RunTimeout time.Duration
	Logger     *slog.Logger
}

type Service struct {
	db         *db.DB
	runs       *revisions.Service
	listen     events.Listener
	publicURL  *url.URL
	runTimeout time.Duration
	logger     *slog.Logger
	steps      *revisions.Steps
	hub        *hub
	poll       time.Duration
}

func New(c Config) *Service {
	if c.DB == nil || c.Runs == nil || c.Listen == nil || c.PublicURL == nil {
		panic("claims.New needs a database, the runs service, a listener and the public URL")
	}
	return &Service{
		db:         c.DB,
		runs:       c.Runs,
		listen:     c.Listen,
		publicURL:  c.PublicURL,
		runTimeout: c.RunTimeout,
		logger:     c.Logger,
		steps:      revisions.NewSteps(c.DB, c.Logger),
		hub:        newHub(),
		poll:       pollEvery,
	}
}

func (s *Service) runTime() time.Duration {
	if s.runTimeout > 0 {
		return s.runTimeout
	}
	return defaultRunTime
}

type Claimer struct {
	Kind   revisions.AssigneeKind
	ID     string
	Person string
	Client string
}

func (c Claimer) claimant() revisions.Claimant {
	return revisions.Claimant{Kind: c.Kind, ID: c.ID, Person: c.Person}
}

type claimerKey struct{}

func WithClaimer(ctx context.Context, c Claimer) context.Context {
	return context.WithValue(ctx, claimerKey{}, c)
}

func ClaimerFrom(ctx context.Context) (Claimer, bool) {
	c, ok := ctx.Value(claimerKey{}).(Claimer)
	return c, ok
}

func (s *Service) eligible(ctx context.Context, c Claimer) error {
	switch c.Kind {
	case revisions.AssignGrant:
		return nil
	case revisions.AssignServiceAccount:
		id, err := db.ParseUUID(c.ID)
		if err != nil {
			return ErrNotAWorker
		}
		account, err := s.db.Queries().GetServiceAccount(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !account.TakesRuns) {
			return ErrNotAWorker
		}
		return err
	}
	return ErrNotAWorker
}

func textOf(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func cleanText(s string, maxRunes int) string {
	s = strings.TrimSpace(strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), ""))
	if utf8.RuneCountInString(s) > maxRunes {
		return string([]rune(s)[:maxRunes])
	}
	return s
}
