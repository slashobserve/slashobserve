package findings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxEvidence       = 4
	MaxWords          = 400
	MaxMarkdownRunes  = 4000
	MaxTitleRunes     = 80
	maxFirstLineRunes = 200
	evidenceWidth     = 12
	evidenceHeight    = 8
	evidenceColumns   = 2
	PinTag            = "finding"
	defaultTitle      = "Finding"
)

var (
	ErrNotFound      = errors.New("finding not found")
	ErrNoDashboard   = errors.New("the finding has no dashboard to pin to")
	ErrNoEvidence    = errors.New("the finding has no evidence panels")
	ErrInvalidCursor = errors.New("invalid cursor")

	ErrFolderNotFound = errors.New("folder not found")
	ErrPlacement      = errors.New("a finding is about a dashboard or filed in a folder, not both")
)

type Author struct{ Kind, ID, Name string }

type Finding struct {
	ID, AgentRequestID, DashboardID, AnnotationID string
	FolderID                                      string
	Title, Markdown                               string
	Evidence                                      []spec.Panel
	From, To, CreatedAt                           time.Time
	RequestTo                                     *time.Time
	CreatedBy                                     Author
	Actor                                         actor.Actor
	Folder                                        *folders.Folder
	Abilities                                     folders.Abilities
}

func Title(text string) string {
	line := firstNonBlankLine(text)
	if line == "" {
		return defaultTitle
	}
	return cutRunes(line, MaxTitleRunes)
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
	doc, err := json.Marshal(map[string]any{
		"specVersion": spec.Version,
		"title":       title,
		"time":        evidenceTime(from, to),
		"panels":      placed,
	})
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
