package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/grounding"
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
	stepSavedAnswer = "Saved the answer"

	CodeFindingTooLong = "finding_too_long"
	codeNotInThisRun   = "not_in_this_run"

	defaultWindow = time.Hour

	descFindingBase     = "Save your answer as a finding and end the run: markdown of at most 400 words answering with the names and numbers that show it, plus, with counts, 1 to 4 evidence panels whose queries return data. Call it once, after checking each evidence query. Open with a plain sentence, no query jargon. Write times in the context's time zone with its label, else in UTC; never mention another product. Give only counts you read; say what you did not check unless the request names a length or a form; before saying a signal is missing, list its metrics scoped to the service; a failed read is not empty. Reads are refused once the run's model-call limit passes without a save, so batch reads and name a refused read in the finding as not read. Cite an observation as a titled /o/<id> link."
	descFindingAskOn    = " A request on Home to change an existing dashboard is made from its own chat: find it with list_dashboards; when exactly one matches, say so and pass its id as ask_on_dashboard_id, else list the candidates by title."
	descFindingFields   = " For an investigation also fill causes, onset (absolute time), impact (with numbers), confidence, ruled_out and next_steps, each in one or two sentences, or set no_incident when nothing was wrong; they must agree with the markdown."
	descSaveFinding     = descFindingBase + descFindingFields + descFindingAskOn
	propFindingMarkdown = `"markdown":{"type":"string","minLength":1,"maxLength":4000,"description":"The finding as Markdown, at most 400 words: a one-sentence answer first, then the evidence unless the request names a length or form."}`
	propFindingTitle    = `"title":{"type":"string","maxLength":120,"description":"Three to eight words naming the answer's subject, such as Checkout 5xx errors by pod; not the question."}`
	propFindingEvidence = `"evidence":{"type":"array","maxItems":4,"items":{"type":"object"},"description":"1 to 4 data panels in ObserveSpec form whose queries returned data; gridPos and ids are set for you."}`
	propFindingFrom     = `"from":{"type":"string","description":"Start of the evidence window: now-1h or an RFC 3339 time."}`
	propFindingTo       = `"to":{"type":"string","description":"End of the evidence window: now or an RFC 3339 time."}`

	propFindingAskOn = `"ask_on_dashboard_id":{"type":"string","description":"On Home: the id of the one existing dashboard the request asks to change."}`

	propFindingSuggestedChange = `"suggested_change":{"type":"string","maxLength":500,"description":"Asked about a dashboard only: the change the person asked for, in their terms, or the nearest safe one; omitted when none is needed, never repeated in markdown; a person with Comment applies it with Revise the dashboard, an agent over MCP."}`
	propFindingObserveWorthy   = `"observe_worthy":{"type":"boolean","description":"True when the answer shows a jump, drop or error spike."}`
	propFindingAnswers         = `"answers":{"type":"array","maxItems":10,"items":{"type":"object","properties":{"quantity":{"type":"string"},"fact":{"type":"string"}},"required":["quantity","fact"]},"description":"When results gave fact ids: each quantity asked, in a few words, with the one fact id answering it, cited in markdown as {{f1}}."}`

	propFindingOnset    = `"onset":{"type":"string","description":"Absolute RFC 3339 time."}`
	propFindingImpact   = `"impact":{"type":"string","maxLength":300}`
	propFindingConf     = `"confidence":{"type":"string","enum":["high","medium","low"]}`
	propFindingRuledOut = `"ruled_out":{"type":"array","maxItems":5,"items":{"type":"string","maxLength":200}}`
	propFindingNext     = `"next_steps":{"type":"array","maxItems":3,"items":{"type":"string","maxLength":200}}`
	propFindingNoInc    = `"no_incident":{"type":"boolean"}`

	propCauseComponent = `{"type":"object","properties":{"kind":{"type":"string","enum":["service","pod","deployment","config","flag","datastore","dependency","node","other"]},"name":{"type":"string","maxLength":120},"namespace":{"type":"string","maxLength":120}},"required":["kind","name"],"additionalProperties":false}`
	propFindingCauses  = `"causes":{"type":"array","maxItems":6,"description":"Each cause the evidence supports, most confident first; none added to be safe. A service failing only on another listed cause's calls sets knock_on_of to that cause's index. The server scores confidence from evidence; estimate is yours.","items":{"type":"object","properties":{` +
		`"component":` + propCauseComponent + `,"kind":{"type":"string","enum":["deploy","config_or_flag","dependency","resource","data","traffic","infrastructure","unknown"]},` +
		`"mechanism":{"type":"string","maxLength":300,"description":"One sentence: how it failed, with the changed value."},` +
		`"trigger":{"type":"object","properties":{"kind":{"type":"string","enum":["feature_flag","config_value","deploy","traffic","dependency","unknown"]},"key":{"type":"string","maxLength":200,"description":"Flag key, setting, version or name exactly as the system spells it."},"value":{"type":"string","maxLength":200}},"required":["kind","key"],"additionalProperties":false},` +
		`"onset":{"type":"string"},"affected":{"type":"array","maxItems":8,"items":` + propCauseComponent + `},"knock_on_of":{"type":"integer","minimum":0,"maximum":5},` +
		`"evidence":{"type":"object","properties":{"facts":{"type":"array","maxItems":12,"items":{"type":"string"}},"trace_ids":{"type":"array","maxItems":4,"items":{"type":"string"}}},"additionalProperties":false},` +
		`"estimate":{"type":"string","enum":["high","medium","low"]}{cause_agent_props}},"required":["component","kind","mechanism"],"additionalProperties":false}}`

	maxSuggestedChangeRunes = 500

	schemaFindingHead       = `{"type":"object","properties":{` + propFindingTitle + `,` + propFindingMarkdown + `,` + propFindingEvidence + `,` + propFindingFrom + `,` + propFindingTo + `,`
	schemaFindingPlain      = propFindingSuggestedChange + `,` + propFindingObserveWorthy + `,` + propFindingAnswers + `{finding_agent_props}`
	schemaFindingStructured = `,` + propFindingCauses + `,` + propFindingOnset + `,` + propFindingImpact + `,` + propFindingConf + `,` + propFindingRuledOut + `,` + propFindingNext + `,` + propFindingNoInc
	schemaFindingEnd        = `},"required":["markdown"],"additionalProperties":false}`

	schemaSaveFinding = schemaFindingHead + propFindingAskOn + `,` + schemaFindingPlain + schemaFindingStructured + schemaFindingEnd

	noteStructuredDropped = "causes, root_cause, onset, impact, confidence, ruled_out, next_steps and no_incident were dropped because this run is not an investigation; the finding is saved without them."
)

var errFindingsNotConfigured = errors.New("findings are not configured")

func init() { registerEntries((*Registry).findingEntries) }

func findingForRun(t Tool, mode Mode, investigation bool) Tool {
	askOn := mode == ModeCreate
	desc, schema := descFindingBase, schemaFindingHead
	if askOn {
		schema += propFindingAskOn + ","
	}
	schema += schemaFindingPlain
	if investigation {
		desc += descFindingFields
		schema += schemaFindingStructured
	}
	if askOn {
		desc += descFindingAskOn
	}
	t.Description, t.InputSchema = desc, json.RawMessage(agentWording.replacer.Replace(schema+schemaFindingEnd))
	return t
}

func (r *Registry) findingEntries() []entry {
	return []entry{
		{Tool{Name: nameSaveFinding, Description: descSaveFinding, InputSchema: json.RawMessage(schemaSaveFinding), AgentOnly: true, Answers: true, MinRole: auth.RoleEditor}, r.saveFinding},
	}
}

type findingInput struct {
	Title    string            `json:"title"`
	Markdown string            `json:"markdown"`
	Evidence []json.RawMessage `json:"evidence"`
	From     string            `json:"from"`
	To       string            `json:"to"`
	AskOn    string            `json:"ask_on_dashboard_id"`

	SuggestedChange string             `json:"suggested_change"`
	ObserveWorthy   bool               `json:"observe_worthy"`
	Answers         []grounding.Answer `json:"answers"`

	findings.StructuredInput
	agentFinding
}

type findingSavedOut struct {
	FindingID string `json:"finding_id"`
	URL       string `json:"url"`
	Replayed  bool   `json:"replayed"`
	Evidence  int    `json:"evidence"`
	Note      string `json:"note,omitempty"`

	Causes []causeOut `json:"causes,omitempty"`
}

type causeOut struct {
	Component  string   `json:"component"`
	Confidence *float64 `json:"confidence"`
	Band       *string  `json:"band"`
	Stated     bool     `json:"stated"`
}

func causesOut(causes []findings.Cause) []causeOut {
	out := make([]causeOut, len(causes))
	for i, c := range causes {
		out[i] = causeOut{Component: c.Component.Text(), Confidence: c.Score, Stated: c.Stated}
		if c.Band != "" {
			out[i].Band = &c.Band
		}
	}
	return out
}

func withLegacyFacts(ctx context.Context, in findingInput) findingInput {
	if _, ok := factsOf(ctx); ok && len(in.Causes) == 0 && in.RootCause != nil {
		in.LegacyFacts = grounding.CitedFacts(in.Markdown, in.Impact, in.RootCause.Summary)
	}
	return in
}

func scoredCauses(ctx context.Context, s *findings.Structured, agent agentFindingFields) string {
	if len(s.Causes) == 0 {
		return ""
	}
	signals, ok := causeSignalsOf(ctx)
	signals, given, ok := agent.scoring(s, signals, ok)
	if !ok {
		return ""
	}
	notes := s.ScoreGiven(signals, given)
	if ledger, ok := factsOf(ctx); ok {
		s.WithQueries(ledger.QueryOf)
	}
	return findings.ScoreNote(notes)
}

func (r *Registry) saveFinding(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Findings == nil {
		return Result{}, errFindingsNotConfigured
	}
	var in findingInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	in = withLegacyFacts(ctx, in)
	in = renderedFinding(ctx, in)
	run, _ := RunFrom(ctx)
	agent := agentFindingOf(in.agentFinding, in.CauseInputs(), run)
	ctx = agent.into(ctx)
	principal, _ := auth.FromContext(ctx)
	requestID, err := optionalUUID(run.RequestID)
	if err != nil {
		return Result{}, fmt.Errorf("run request id: %w", err)
	}
	dashboardID, err := optionalUUID(run.DashboardID)
	if err != nil {
		return Result{}, fmt.Errorf("run dashboard id: %w", err)
	}
	from, to := runWindow(ctx, run)
	extras, res, ok := checkedExtras(in, run)
	if !ok {
		return res, nil
	}
	suggested := extras.suggested
	in.Markdown = markdownWithoutSuggestedChange(in.Markdown, suggested)
	save, res, ok, err := r.checkFinding(ctx, principal, in, from, to, findings.TitleFrom(in.Title, run.Investigation))
	if err != nil || !ok {
		return res, err
	}
	if res, wrong := zoneMismatch(save.Markdown, run.TimeZone, save.From, save.To); wrong {
		return res, nil
	}
	save.AgentRequestID, save.DashboardID, save.AskDashboardID = requestID, dashboardID, extras.askOn
	save.SuggestedChange, save.ObserveWorthy = suggested, in.ObserveWorthy
	scoreNote := scoredCauses(ctx, &extras.structured, agent)
	save.Structured = extras.structured
	save.Unverified = unverifiedOf(ctx)
	if agent.hasUnverify {
		save.Unverified = agent.unverified
	}
	save.AsOf = run.asOf()
	save.ClaimEpoch = run.ClaimEpoch
	f, replayed, err := r.deps.Findings.Save(ctx, principal, save)
	if errors.Is(err, findings.ErrNotFound) {
		return toolError(codeNotFound, "dashboard not found"), nil
	}
	if res, ok := restrictedRefusal(ctx, err); ok {
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	out := findingSavedOut{FindingID: f.ID, URL: r.findingURL(f), Replayed: replayed, Evidence: len(f.Evidence), Causes: causesOut(f.Structured.Causes)}
	if extras.droppedStructured {
		out.Note = noteStructuredDropped
	} else {
		out.Note = scoreNote
	}
	return success(stepSavedAnswer, out), nil
}

func runWindow(ctx context.Context, run Run) (time.Time, time.Time) {
	if run.From.IsZero() || run.To.IsZero() {
		to := Now(ctx)
		return to.Add(-defaultWindow), to
	}
	return run.From, run.To
}

func renderedFinding(ctx context.Context, in findingInput) findingInput {
	ledger, ok := factsOf(ctx)
	if !ok {
		return in
	}
	render := func(text string) string {
		out, _ := ledger.Render(text)
		return out
	}
	in.Title, in.Markdown, in.Impact = render(in.Title), render(in.Markdown), render(in.Impact)
	if in.RootCause != nil {
		cause := *in.RootCause
		cause.Summary = render(cause.Summary)
		in.RootCause = &cause
	}
	if len(in.Causes) > 0 {
		causes := slices.Clone(in.Causes)
		for i := range causes {
			causes[i].Mechanism = render(causes[i].Mechanism)
		}
		in.Causes = causes
	}
	in.RuledOut = renderedAll(in.RuledOut, render)
	in.NextSteps = renderedAll(in.NextSteps, render)
	return in
}

func renderedAll(texts []string, render func(string) string) []string {
	if len(texts) == 0 {
		return texts
	}
	out := make([]string, len(texts))
	for i, text := range texts {
		out[i] = render(text)
	}
	return out
}

func (r *Registry) findingURL(f findings.Finding) string {
	if f.Saved() {
		return links.Observation(r.deps.PublicURL, f.ObservationID)
	}
	return links.Chat(r.deps.PublicURL, f.AgentRequestID)
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

type findingExtras struct {
	askOn             pgtype.UUID
	suggested         string
	structured        findings.Structured
	droppedStructured bool
}

func checkedExtras(in findingInput, run Run) (findingExtras, Result, bool) {
	var out findingExtras
	var res Result
	var ok bool
	if out.askOn, res, ok = askOnTarget(in.AskOn, run); !ok {
		return findingExtras{}, res, false
	}
	if out.suggested, res, ok = suggestedChange(in.SuggestedChange, run); !ok {
		return findingExtras{}, res, false
	}
	if !run.Investigation {
		out.droppedStructured = in.Provided()
		return out, Result{}, true
	}
	structured, err := in.Check()
	if err != nil {
		return findingExtras{}, invalid("%s", err), false
	}
	out.structured = structured
	return out, Result{}, true
}

func suggestedChange(raw string, run Run) (string, Result, bool) {
	text := strings.TrimSpace(raw)
	switch {
	case text == "":
		return "", Result{}, true
	case run.Mode != ModeInvestigate || run.DashboardID == "":
		return "", invalid("suggested_change applies only to a question about a dashboard; leave it out here"), false
	case utf8.RuneCountInString(text) > maxSuggestedChangeRunes:
		return "", invalid("suggested_change takes at most %d characters; describe the change in one sentence", maxSuggestedChangeRunes), false
	case strings.ContainsFunc(text, unicode.IsControl):
		return "", invalid("suggested_change is one plain sentence with no line breaks"), false
	}
	return text, Result{}, true
}

var (
	suggestedChangeLabel = regexp.MustCompile(`(?i)^ {0,3}[*_]{0,3}(?:suggested changes?|suggested_change)[*_]{0,3}\s*:`)
	codeFence            = regexp.MustCompile("^ {0,3}(```+|~~~+)")
)

func markdownWithoutSuggestedChange(markdown, suggested string) string {
	if suggested == "" {
		return markdown
	}
	return withoutSuggestedChange(markdown)
}

func withoutSuggestedChange(markdown string) string {
	lines := strings.Split(markdown, "\n")
	cut, fence := -1, ""
	for i, line := range lines {
		if m := codeFence.FindStringSubmatch(line); m != nil {
			switch {
			case fence == "":
				fence = m[1][:1]
			case strings.HasPrefix(m[1], fence):
				fence = ""
			}
			continue
		}
		if fence == "" && suggestedChangeLabel.MatchString(line) {
			cut = i
		}
	}
	if cut < 0 {
		return markdown
	}
	kept := lines[:cut]
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) == 0 {
		return markdown
	}
	return strings.Join(kept, "\n")
}

func (r *Registry) checkFinding(ctx context.Context, principal auth.Principal, in findingInput, defaultFrom, defaultTo time.Time, title string) (findings.SaveInput, Result, bool, error) {
	markdown, res, ok := findingMarkdown(in.Markdown, ReadsRefused(ctx), UncheckedReads(ctx))
	if !ok {
		return findings.SaveInput{}, res, false, nil
	}
	if len(in.Evidence) > findings.MaxEvidence {
		return findings.SaveInput{}, invalid("evidence takes at most %d panels", findings.MaxEvidence), false, nil
	}
	now := Now(ctx)
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

var uncheckedReasons = map[string]string{
	string(queryproxy.CodeQueryTimeout):    "a read timed out",
	string(queryproxy.CodeDatasourceDown):  "the datasource did not answer",
	string(queryproxy.CodeTooManyRequests): "the server was too busy to ask it",
}

func uncheckedCaveat(unchecked []Unchecked) string {
	if len(unchecked) == 0 {
		return ""
	}
	parts := make([]string, len(unchecked))
	for i, u := range unchecked {
		parts[i] = fmt.Sprintf("%s (%s)", clip(u.Datasource, maxStepNameBytes), cmp.Or(uncheckedReasons[u.Code], "a read failed"))
	}
	return "\n\n_Could not check " + strings.Join(parts, ", ") + ": those reads did not finish, so this answer does not show that their data is missing._"
}

func findingMarkdown(raw string, refused int, unchecked []Unchecked) (string, Result, bool) {
	markdown := strings.TrimSpace(raw)
	if markdown == "" {
		return "", invalid("markdown is required"), false
	}
	caveat := readsCaveat(refused) + uncheckedCaveat(unchecked)
	maxWords := findings.MaxWords - findings.Words(caveat)
	maxRunes := findings.MaxMarkdownRunes - utf8.RuneCountInString(caveat)
	if n := findings.Words(markdown); n > maxWords {
		res := toolError(CodeFindingTooLong, fmt.Sprintf("the finding is %d words and the limit is %d; shorten the text by %s, keeping every figure the question asks for", n, maxWords, plural(n-maxWords, "word", "words")))
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
	issues, err := r.deps.Runner.CheckPanels(ctx, d, now, queryproxy.UserFor(principal), ids)
	if err != nil {
		return nil, Result{}, false, err
	}
	if len(issues) > 0 {
		return nil, evidenceRejected(issues, evidencePanelPointers(d)), false, nil
	}
	return d, Result{}, true, nil
}

func evidencePanelPointers(d *spec.Dashboard) map[string]string {
	pointers := map[string]string{}
	for _, e := range d.DataPanels() {
		pointers[e.Pointer] = e.Panel.ID
	}
	return pointers
}

func evidenceRejected(issues []spec.Issue, pointers map[string]string) Result {
	out := rejectionOf(issues, pointers)
	out.Message = evidenceRepairMessage(issues)
	return failure("Evidence rejected: "+plural(len(issues), "issue", "issues")+issueCodes(issues), out)
}

func evidenceRepairMessage(issues []spec.Issue) string {
	lead := "evidence-N is the Nth panel you sent. "
	if allTransient(issues) {
		return lead + "A datasource did not answer in time; that is not a fault in the panels. Send the call again without the panels in failing_panels and say in the text that their data was not read."
	}
	return lead + repairLead + " " + repairQuery + " Or leave a panel out and say in the text what you could not check."
}

func allTransient(issues []spec.Issue) bool {
	return len(issues) > 0 && !slices.ContainsFunc(issues, func(i spec.Issue) bool { return !CouldNotCheck(string(i.Code)) })
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
