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
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameSaveFinding = "save_finding"

	codeTooLong      = "finding_too_long"
	codeNotInThisRun = "not_in_this_run"

	defaultWindow = time.Hour

	descSaveFinding     = "Save your answer as a finding and end the run: markdown of at most 400 words that answers with the names and numbers that show it, plus up to 4 evidence panels whose queries return data in the window. Call it exactly once, after you have checked each evidence query with query_range or query_instant. Open with one plain sentence, no query jargon. Write times as UTC with the word UTC, and never mention another product. Give only counts you read and say what you did not check; before saying a signal is missing, list its metrics with a filter for the service. Reads are refused once the run's limit of model calls passes without a save, so run every read together, and put a read you were refused in the finding as not read. Cite an earlier observation you read by its title and /o/<id> link. A request on Home to change a dashboard that already exists cannot be done there: find the dashboard with list_dashboards, and when exactly one matches, say so in the finding and pass its id as ask_on_dashboard_id; when none or several match, list the candidates by title and pass none."
	propFindingMarkdown = `"markdown":{"type":"string","minLength":1,"maxLength":4000,"description":"The finding as Markdown, at most 400 words: a one-sentence answer first, then the evidence."}`
	propFindingEvidence = `"evidence":{"type":"array","maxItems":4,"items":{"type":"object"},"description":"Up to 4 data panels in ObserveSpec form whose queries returned data; gridPos and ids are set for you."}`
	propFindingFrom     = `"from":{"type":"string","description":"Start of the window the evidence shows, such as now-1h or an RFC 3339 time."}`
	propFindingTo       = `"to":{"type":"string","description":"End of the window the evidence shows, such as now or an RFC 3339 time."}`

	propFindingAskOn = `"ask_on_dashboard_id":{"type":"string","description":"On a request on Home that names no dashboard: the id of the one existing dashboard the request is about, so the change can be asked for on that dashboard."}`

	schemaSaveFinding = `{"type":"object","properties":{` + propFindingMarkdown + `,` + propFindingEvidence + `,` + propFindingFrom + `,` + propFindingTo + `,` + propFindingAskOn + `},"required":["markdown"],"additionalProperties":false}`
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
	AskOn    string            `json:"ask_on_dashboard_id"`
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
	askOn, res, ok := askOnTarget(in.AskOn, run)
	if !ok {
		return res, nil
	}
	save, res, ok, err := r.checkFinding(ctx, principal, in, from, to, findings.Title(run.Prompt))
	if err != nil || !ok {
		return res, err
	}
	save.AgentRequestID, save.DashboardID, save.AskDashboardID = requestID, dashboardID, askOn
	f, replayed, err := r.deps.Findings.Save(ctx, principal, save)
	if errors.Is(err, findings.ErrNotFound) {
		return toolError(codeNotFound, "dashboard not found"), nil
	}
	if err != nil {
		return Result{}, err
	}
	return success("Saved finding "+f.ID, findingSavedOut{Code: "saved", FindingID: f.ID, URL: links.Observation(r.deps.PublicURL, f.ObservationID), Replayed: replayed, Evidence: len(f.Evidence)}), nil
}

func askOnTarget(raw string, run Run) (pgtype.UUID, Result, bool) {
	if raw == "" {
		return pgtype.UUID{}, Result{}, true
	}
	if run.Mode != ModeCreate {
		return pgtype.UUID{}, invalid("ask_on_dashboard_id applies only to a request on Home that names no dashboard"), false
	}
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("ask_on_dashboard_id must be the id of a dashboard from list_dashboards"), false
	}
	return id, Result{}, true
}

func (r *Registry) checkFinding(ctx context.Context, principal auth.Principal, in findingInput, defaultFrom, defaultTo time.Time, title string) (findings.SaveInput, Result, bool, error) {
	markdown, res, ok := findingMarkdown(in.Markdown, ReadsRefused(ctx))
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

func readsCaveat(refused int) string {
	if refused == 0 {
		return ""
	}
	return fmt.Sprintf("\n\n_Reading stopped early: %s refused after the run reached its limit of model calls, so the counts above may be partial. Ask again about the part that matters to read it fully._", plural(refused, "read was", "reads were"))
}

func findingMarkdown(raw string, refused int) (string, Result, bool) {
	markdown := strings.TrimSpace(raw)
	if markdown == "" {
		return "", invalid("markdown is required"), false
	}
	caveat := readsCaveat(refused)
	maxWords := findings.MaxWords - findings.Words(caveat)
	maxRunes := findings.MaxMarkdownRunes - utf8.RuneCountInString(caveat)
	if n := findings.Words(markdown); n > maxWords {
		res := toolError(codeTooLong, fmt.Sprintf("the finding is %d words; keep it to %d words and move detail into evidence panels", n, maxWords))
		res.Step = fmt.Sprintf("Finding was %d words, over the %d word limit; the agent must shorten it", n, maxWords)
		return "", res, false
	}
	switch _, textErr := revisions.CheckPrompt("markdown", markdown); {
	case utf8.RuneCountInString(markdown) > maxRunes:
		return "", invalid("markdown must be at most %d characters", maxRunes), false
	case textErr != nil:
		return "", invalid("%s", textErr), false
	case containsHTML(markdown):
		return "", invalid("markdown must be plain Markdown without HTML"), false
	}
	return markdown + caveat, Result{}, true
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
	return r.evidenceDashboardWith(ctx, principal, save, evidence, nil, false, now)
}

func (r *Registry) evidenceDashboardWith(ctx context.Context, principal auth.Principal, save findings.SaveInput, evidence []json.RawMessage, variables []spec.Variable, takesVariables bool, now time.Time) (*spec.Dashboard, Result, bool, error) {
	ids := make([]string, len(evidence))
	for i, raw := range evidence {
		if res, ok := evidencePanelAllowed(i, raw); !ok {
			return nil, res, false, nil
		}
		ids[i] = fmt.Sprintf("evidence-%d", i+1)
	}
	d, err := findings.EvidenceSpecWith(save.Title, save.From, save.To, evidence, variables)
	var invalidSpec *spec.ValidationError
	switch {
	case errors.As(err, &invalidSpec):
		if name, ok := observations.MissingVariable(invalidSpec.Issues); ok && takesVariables {
			return nil, toolError(observations.CodeVariableMissing, fmt.Sprintf("the panel uses the variable %q and no value was given for it; send its values in variables", name)), false, nil
		}
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
