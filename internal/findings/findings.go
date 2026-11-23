package findings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/usertext"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxEvidence       = 4
	MaxWords          = 400
	MaxMarkdownRunes  = 4000
	maxFirstLineRunes = 200
	evidenceWidth     = 12
	evidenceHeight    = 8
	evidenceColumns   = 2
	PinTag            = "finding"
	defaultTitle      = "Finding"
	answerTitle       = "Answer"
)

var (
	ErrNotFound      = errors.New("finding not found")
	ErrNoDashboard   = errors.New("the finding has no dashboard to pin to")
	ErrNoEvidence    = errors.New("the finding has no evidence panels")
	ErrInvalidCursor = errors.New("invalid cursor")

	ErrFolderNotFound = errors.New("folder not found")
	ErrPlacement      = errors.New("a finding is about a dashboard or filed in a folder, not both")
	ErrRequesterOnly  = errors.New("only the one who asked can use the answer of a run that is not an observation")
	ErrNotSaved       = errors.New("the answer is not an observation yet; save it as an observation first")
	ErrAskPlacement   = errors.New("a finding about a dashboard has no other dashboard to ask on")
)

type Conclusion struct {
	RunID, DashboardID, FolderID pgtype.UUID
	Into                         pgtype.UUID
	Title, Markdown              string
	Evidence                     []spec.Panel
	From, To                     time.Time
}

type Observations interface {
	WriteConclusion(ctx context.Context, q *dbsqlc.Queries, by auth.Principal, c Conclusion) (id pgtype.UUID, created bool, err error)
	Placement(ctx context.Context, by auth.Principal, observationID pgtype.UUID) (folders.Folder, folders.Abilities, error)
}

type ThreadReader func(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error

type AnswerReadRecorder func(ctx context.Context, runID pgtype.UUID)

type Author struct{ Kind, ID, Name string }

type Finding struct {
	ID, AgentRequestID, DashboardID, AnnotationID string
	ObservationID, FolderID                       string
	AskOnID, AskOnTitle                           string
	Title, Markdown                               string
	SuggestedChange                               string
	ObserveWorthy                                 bool
	Restricted                                    bool
	Evidence                                      []spec.Panel
	From, To, CreatedAt                           time.Time
	RequestTo, AsOf                               *time.Time
	SharedIn                                      pgtype.UUID
	CreatedBy                                     Author
	RequestedBy                                   Author
	Actor                                         actor.Actor
	Folder                                        *folders.Folder
	Abilities                                     folders.Abilities
	observationHidden                             bool
}

func (f Finding) Saved() bool {
	return f.ObservationID != ""
}

func Title(text string) string {
	if title := usertext.ShortTitle(text); title != "" {
		return title
	}
	return defaultTitle
}

func TitleFrom(given string, investigation bool) string {
	switch title := usertext.ShortTitle(given); {
	case title != "":
		return title
	case investigation:
		return defaultTitle
	}
	return answerTitle
}

func FirstLine(markdown string) string {
	line := strings.TrimLeft(firstNonBlankLine(markdown), "#>-* ")
	if line == "" {
		return Title(markdown)
	}
	return cutRunes(line, maxFirstLineRunes)
}

func Words(markdown string) int {
	return len(strings.Fields(markdown))
}

func PinText(f Finding) string {
	return FirstLine(f.Markdown) + " " + links.Finding(nil, f.ID)
}

func firstNonBlankLine(text string) string {
	for line := range strings.Lines(text) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:n]), " \t")
}

func EvidenceSpec(title string, from, to time.Time, panels []json.RawMessage) (*spec.Dashboard, error) {
	return EvidenceSpecWith(title, from, to, panels, nil)
}

func EvidenceSpecWith(title string, from, to time.Time, panels []json.RawMessage, variables []spec.Variable) (*spec.Dashboard, error) {
	placed := make([]map[string]json.RawMessage, len(panels))
	for i, raw := range panels {
		var panel map[string]json.RawMessage
		if err := json.Unmarshal(raw, &panel); err != nil || panel == nil {
			return nil, fmt.Errorf("evidence[%d] must be a JSON object", i)
		}
		id, err := json.Marshal(evidenceID(i))
		if err != nil {
			return nil, err
		}
		grid, err := json.Marshal(evidenceGrid(i))
		if err != nil {
			return nil, err
		}
		panel["id"], panel["gridPos"] = id, grid
		placed[i] = panel
	}
	document := map[string]any{
		"specVersion": spec.Version,
		"title":       title,
		"time":        evidenceTime(from, to),
		"panels":      placed,
	}
	if len(variables) > 0 {
		document["variables"] = variables
	}
	doc, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	return spec.Parse(doc)
}

func (f Finding) Spec() *spec.Dashboard {
	return &spec.Dashboard{
		SpecVersion: spec.Version,
		Title:       f.Title,
		Description: FirstLine(f.Markdown),
		Time:        evidenceTime(f.From, f.To),
		Panels:      f.Evidence,
	}
}

func evidenceID(i int) string {
	return "evidence-" + strconv.Itoa(i+1)
}

func evidenceGrid(i int) spec.GridPos {
	return spec.GridPos{X: (i % evidenceColumns) * evidenceWidth, Y: (i / evidenceColumns) * evidenceHeight, W: evidenceWidth, H: evidenceHeight}
}

func evidenceTime(from, to time.Time) spec.TimeRange {
	return spec.TimeRange{From: from.UTC().Format(time.RFC3339Nano), To: to.UTC().Format(time.RFC3339Nano)}
}
