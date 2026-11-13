package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameSaveFinding = "save_finding"

	codeTooLong      = "finding_too_long"
	codeNotInThisRun = "not_in_this_run"

	defaultWindow = time.Hour

	descSaveFinding     = "Save your answer as a finding and end the run: markdown of at most 400 words that answers with the names and numbers that show it, plus up to 4 evidence panels whose queries return data in the window. Call it exactly once, after you have checked each evidence query with query_range or query_instant."
	propFindingMarkdown = `"markdown":{"type":"string","minLength":1,"maxLength":4000,"description":"The finding as Markdown, at most 400 words: a one-sentence answer first, then the evidence."}`
	propFindingEvidence = `"evidence":{"type":"array","maxItems":4,"items":{"type":"object"},"description":"Up to 4 data panels in ObserveSpec form whose queries returned data; gridPos and ids are set for you."}`
	propFindingFrom     = `"from":{"type":"string","description":"Start of the window the evidence shows, such as now-1h or an RFC 3339 time."}`
	propFindingTo       = `"to":{"type":"string","description":"End of the window the evidence shows, such as now or an RFC 3339 time."}`

	schemaSaveFinding = `{"type":"object","properties":{` + propFindingMarkdown + `,` + propFindingEvidence + `,` + propFindingFrom + `,` + propFindingTo + `},"required":["markdown"],"additionalProperties":false}`
)

var errFindingsNotConfigured = errors.New("findings are not configured")

func init() { registerEntries((*Registry).findingEntries) }

func (r *Registry) findingEntries() []entry {
	return []entry{
		{Tool{Name: nameSaveFinding, Description: descSaveFinding, InputSchema: json.RawMessage(schemaSaveFinding), AgentOnly: true, Answers: true, MinRole: auth.RoleEditor}, r.saveFinding},
	}
}

type findingInput struct {
	Markdown string            `json:"markdown"`
	Evidence []json.RawMessage `json:"evidence"`
	From     string            `json:"from"`
	To       string            `json:"to"`
}

type findingSavedOut struct {
	Code      string `json:"code"`
	FindingID string `json:"finding_id"`
	URL       string `json:"url"`
	Replayed  bool   `json:"replayed"`
	Evidence  int    `json:"evidence"`
}

func (r *Registry) saveFinding(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Findings == nil {
		return Result{}, errFindingsNotConfigured
	}
	var in findingInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	run, _ := RunFrom(ctx)
	principal, _ := auth.FromContext(ctx)
	requestID, err := optionalUUID(run.RequestID)
	if err != nil {
		return Result{}, fmt.Errorf("run request id: %w", err)
	}
	dashboardID, err := optionalUUID(run.DashboardID)
	if err != nil {
		return Result{}, fmt.Errorf("run dashboard id: %w", err)
	}
	from, to := run.From, run.To
	if from.IsZero() || to.IsZero() {
		to = time.Now()
		from = to.Add(-defaultWindow)
	}
	save, res, ok, err := r.checkFinding(ctx, principal, in, from, to, findings.Title(run.Prompt))
	if err != nil || !ok {
		return res, err
	}
	save.AgentRequestID, save.DashboardID = requestID, dashboardID
	f, replayed, err := r.deps.Findings.Save(ctx, principal, save)
	if errors.Is(err, findings.ErrNotFound) {
		return toolError(codeNotFound, "dashboard not found"), nil
	}
	if err != nil {
		return Result{}, err
	}
	return success("Saved finding "+f.ID, findingSavedOut{Code: "saved", FindingID: f.ID, URL: links.Observation(r.deps.PublicURL, f.ObservationID), Replayed: replayed, Evidence: len(f.Evidence)}), nil
}

func (r *Registry) checkFinding(ctx context.Context, principal auth.Principal, in findingInput, defaultFrom, defaultTo time.Time, title string) (findings.SaveInput, Result, bool, error) {
	markdown, res, ok := findingMarkdown(in.Markdown)
	if !ok {
		return findings.SaveInput{}, res, false, nil
	}
	if len(in.Evidence) > findings.MaxEvidence {
		return findings.SaveInput{}, invalid("evidence takes at most %d panels", findings.MaxEvidence), false, nil
	}
	now := time.Now()
	from, to, res, ok := findingWindow(in, defaultFrom, defaultTo, now)
	if !ok {
		return findings.SaveInput{}, res, false, nil
	}
	save := findings.SaveInput{Title: title, Markdown: markdown, From: from, To: to}
	if len(in.Evidence) == 0 {
		return save, Result{}, true, nil
	}
	d, res, ok, err := r.evidenceDashboard(ctx, principal, save, in.Evidence, now)
	if err != nil || !ok {
		return findings.SaveInput{}, res, false, err
	}
	save.Evidence = d.Panels
	return save, Result{}, true, nil
}

func findingMarkdown(raw string) (string, Result, bool) {
	markdown := strings.TrimSpace(raw)
	if markdown == "" {
		return "", invalid("markdown is required"), false
	}
	if n := findings.Words(markdown); n > findings.MaxWords {
		return "", toolError(codeTooLong, fmt.Sprintf("the finding is %d words; keep it to %d words and move detail into evidence panels", n, findings.MaxWords)), false
	}
	switch _, clean := revisions.CleanPrompt(markdown); {
	case utf8.RuneCountInString(markdown) > findings.MaxMarkdownRunes:
		return "", invalid("markdown must be at most %d characters", findings.MaxMarkdownRunes), false
	case !clean:
		return "", invalid("markdown must not contain control or bidirectional override characters"), false
	case containsHTML(markdown):
		return "", invalid("markdown must be plain Markdown without HTML"), false
	}
	return markdown, Result{}, true
}

func findingWindow(in findingInput, defaultFrom, defaultTo, now time.Time) (time.Time, time.Time, Result, bool) {
	switch {
	case (in.From == "") != (in.To == ""):
		return time.Time{}, time.Time{}, invalid("from and to go together"), false
	case in.From == "":
		return defaultFrom, defaultTo, Result{}, true
	}
	window, res, ok := resolveRange(in.From, in.To, now)
	return window.From, window.To, res, ok
}

func (r *Registry) evidenceDashboard(ctx context.Context, principal auth.Principal, save findings.SaveInput, evidence []json.RawMessage, now time.Time) (*spec.Dashboard, Result, bool, error) {
	ids := make([]string, len(evidence))
	for i, raw := range evidence {
		if res, ok := evidencePanelAllowed(i, raw); !ok {
			return nil, res, false, nil
		}
		ids[i] = fmt.Sprintf("evidence-%d", i+1)
	}
	d, err := findings.EvidenceSpec(save.Title, save.From, save.To, evidence)
	var invalidSpec *spec.ValidationError
	switch {
	case errors.As(err, &invalidSpec):
		return nil, suggestionRejected(invalidSpec.Issues), false, nil
	case err != nil:
		return nil, Result{}, false, err
	}
	if issues := importOnlyIssues(d, nil); len(issues) > 0 {
		return nil, suggestionRejected(issues), false, nil
	}
	canonical, err := json.Marshal(d)
	if err != nil {
		return nil, Result{}, false, err
	}
	if issues := spec.NULIssues(canonical); len(issues) > 0 {
		return nil, suggestionRejected(issues), false, nil
	}
	issues, err := r.deps.Runner.CheckPanels(ctx, d, now, queryproxy.UserFor(principal), ids)
	if err != nil {
		return nil, Result{}, false, err
	}
	if len(issues) > 0 {
		return nil, suggestionRejected(issues), false, nil
	}
	return d, Result{}, true, nil
}

func evidencePanelAllowed(i int, raw json.RawMessage) (Result, bool) {
	if len(raw) > spec.MaxSuggestedPanelBytes {
		return toolError(codeTooLarge, fmt.Sprintf("evidence[%d] is %d bytes, over the %d byte limit", i, len(raw), spec.MaxSuggestedPanelBytes)), false
	}
	var head struct {
		Type spec.PanelType `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return invalid("evidence[%d] must be a JSON object", i), false
	}
	if !head.Type.IsData() {
		kind := string(head.Type)
		if kind == "" {
			kind = "a panel without a type"
		}
		return invalid("evidence[%d] must be a data panel with queries, not %s", i, kind), false
	}
	return Result{}, true
}

func optionalUUID(raw string) (pgtype.UUID, error) {
	if raw == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(raw)
}
