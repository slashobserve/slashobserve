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
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameSaveFinding   = "save_finding"
	nameCreateFinding = "create_finding"
	nameListFindings  = "list_findings"
	nameMoveFinding   = "move_finding"

	codeTooLong      = "finding_too_long"
	codeNotInThisRun = "not_in_this_run"

	defaultListFindings = 20
	maxListFindings     = 50
	maxFindingOutBytes  = 4000
	defaultWindow       = time.Hour

	descSaveFinding   = "Save your answer as a finding and end the run: markdown of at most 400 words that answers with the names and numbers that show it, plus up to 4 evidence panels whose queries return data in the window. Call it exactly once, after you have checked each evidence query with query_range or query_instant."
	descCreateFinding = "Save an investigation result as a finding: markdown of at most 400 words and up to 4 evidence panels, each checked by running its query. Returns the finding with its url."
	descListFindings  = "List findings, newest first: those on one dashboard with dashboard_id, {findings_folder_scope}else your own. Page with next_cursor."

	descMoveFinding = "Move a finding without a dashboard into another folder, which shares it with that folder's readers. Needs Edit on the finding and on the folder; a finding on a dashboard follows the dashboard."

	schemaMoveFinding = `{"type":"object","properties":{"finding_id":{"type":"string","format":"uuid","description":"The finding to move, from list_findings."},"folder_id":{"type":"string","format":"uuid","description":"The folder to move it into, from list_folders. Not My Dashboards."}},"required":["finding_id","folder_id"],"additionalProperties":false}`

	propFindingsFolderID = `"folder_id":{"type":"string","format":"uuid","description":"Only findings filed in this folder, from list_folders."}`

	propFindingMarkdown = `"markdown":{"type":"string","minLength":1,"maxLength":4000,"description":"The finding as Markdown, at most 400 words: a one-sentence answer first, then the evidence."}`
	propFindingEvidence = `"evidence":{"type":"array","maxItems":4,"items":{"type":"object"},"description":"Up to 4 data panels in ObserveSpec form whose queries returned data; gridPos and ids are set for you."}`
	propFindingFrom     = `"from":{"type":"string","description":"Start of the window the evidence shows, such as now-1h or an RFC 3339 time."}`
	propFindingTo       = `"to":{"type":"string","description":"End of the window the evidence shows, such as now or an RFC 3339 time."}`

	schemaSaveFinding   = `{"type":"object","properties":{` + propFindingMarkdown + `,` + propFindingEvidence + `,` + propFindingFrom + `,` + propFindingTo + `},"required":["markdown"],"additionalProperties":false}`
	schemaCreateFinding = `{"type":"object","properties":{` + propFindingMarkdown + `,` + propFindingEvidence + `,` + propFindingFrom + `,` + propFindingTo + `,"dashboard_id":{"type":"string","format":"uuid","description":"The dashboard the finding is about, from list_dashboards."},"folder_id":{"type":"string","format":"uuid","description":"The folder to file a finding without a dashboard in, from list_folders. Default your My Dashboards, else General. Not with dashboard_id."},"title":{"type":"string","minLength":1,"maxLength":80,"description":"A short title. Default the first line of markdown."}},"required":["markdown"],"additionalProperties":false}`
	schemaListFindings  = `{"type":"object","properties":{"dashboard_id":{"type":"string","format":"uuid","description":"Only findings on this dashboard. Default your own findings."}{findings_folder_prop},"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Findings per page. Default 20."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
)

var errFindingsNotConfigured = errors.New("findings are not configured")

func init() { registerEntries((*Registry).findingEntries) }

func (r *Registry) findingEntries() []entry {
	return []entry{
		{Tool{Name: nameSaveFinding, Description: descSaveFinding, InputSchema: json.RawMessage(schemaSaveFinding), AgentOnly: true, Answers: true, MinRole: auth.RoleEditor}, r.saveFinding},
		{Tool{Name: nameCreateFinding, Description: descCreateFinding, InputSchema: json.RawMessage(schemaCreateFinding), MCPOnly: true, MinRole: auth.RoleEditor}, r.createFinding},
		{Tool{Name: nameListFindings, Description: descListFindings, InputSchema: json.RawMessage(schemaListFindings), ReadOnly: true, MinRole: auth.RoleViewer}, r.listFindings},
		{Tool{Name: nameMoveFinding, Description: descMoveFinding, InputSchema: json.RawMessage(schemaMoveFinding), MCPOnly: true, MinRole: auth.RoleViewer}, r.moveFinding},
	}
}

type findingInput struct {
	Markdown string            `json:"markdown"`
	Evidence []json.RawMessage `json:"evidence"`
	From     string            `json:"from"`
	To       string            `json:"to"`
}

type createFindingInput struct {
	findingInput
	DashboardID string  `json:"dashboard_id"`
	FolderID    string  `json:"folder_id"`
	Title       *string `json:"title"`
}

type findingSavedOut struct {
	Code      string `json:"code"`
	FindingID string `json:"finding_id"`
	URL       string `json:"url"`
	Replayed  bool   `json:"replayed"`
	Evidence  int    `json:"evidence"`
}

type findingItemOut struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	DashboardID       *string  `json:"dashboard_id"`
	FolderID          *string  `json:"folder_id"`
	Markdown          string   `json:"markdown"`
	MarkdownTruncated bool     `json:"markdown_truncated,omitempty"`
	Evidence          []string `json:"evidence"`
	From              string   `json:"from"`
	To                string   `json:"to"`
	CreatedAt         string   `json:"created_at"`
	URL               string   `json:"url"`
}

type findingListOut struct {
	Findings   []findingItemOut `json:"findings"`
	NextCursor *string          `json:"next_cursor"`
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
	return success("Saved finding "+f.ID, findingSavedOut{Code: "saved", FindingID: f.ID, URL: links.Finding(r.deps.PublicURL, f.ID), Replayed: replayed, Evidence: len(f.Evidence)}), nil
}

func (r *Registry) findingPlacement(ctx context.Context, dashboard, folder string) (dashboardID, folderID pgtype.UUID, res Result, ok bool, err error) {
	if dashboard != "" && folder != "" {
		return dashboardID, folderID, invalid("dashboard_id and folder_id cannot go together: a finding is about a dashboard or filed in a folder"), false, nil
	}
	if dashboard != "" {
		id, res, ok := parseDashboardID(dashboard)
		if !ok {
			return dashboardID, folderID, res, false, nil
		}
		if res, ok, err := r.viewableDashboard(ctx, id, "dashboard not found"); err != nil || !ok {
			return dashboardID, folderID, res, false, err
		}
		dashboardID = id
	}
	folderID, res, ok = parseFolderID(folder)
	return dashboardID, folderID, res, ok, nil
}

func (r *Registry) createFinding(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Findings == nil {
		return Result{}, errFindingsNotConfigured
	}
	var in createFindingInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	dashboardID, folderID, res, ok, err := r.findingPlacement(ctx, in.DashboardID, in.FolderID)
	if err != nil || !ok {
		return res, err
	}
	title := findings.Title(in.Markdown)
	if in.Title != nil {
		cleaned, ok := findingTitle(*in.Title)
		if !ok {
			return invalid("title must be 1 to %d characters on one line", findings.MaxTitleRunes), nil
		}
		title = cleaned
	}
	to := time.Now()
	save, res, ok, err := r.checkFinding(ctx, principal, in.findingInput, to.Add(-defaultWindow), to, title)
	if err != nil || !ok {
		return res, err
	}
	save.DashboardID, save.FolderID = dashboardID, folderID
	f, _, err := r.deps.Findings.Save(ctx, principal, save)
	switch {
	case errors.Is(err, findings.ErrNotFound):
		return toolError(codeNotFound, "dashboard not found"), nil
	case errors.Is(err, findings.ErrFolderNotFound):
		return toolError(codeNotFound, "folder not found"), nil
	case err != nil:
		return Result{}, err
	}
	return success("Saved finding "+f.ID, findings.JSONOf(f, links.Finding(r.deps.PublicURL, f.ID))), nil
}

type findingMovedOut struct {
	findingItemOut
	Folder string `json:"folder"`
}

func (r *Registry) moveFinding(ctx context.Context, input json.RawMessage) (Result, error) {
	if _, scoped := RunFrom(ctx); scoped {
		return invalid("%s is for MCP clients; a run saves only dashboards or a finding", nameMoveFinding), nil
	}
	if r.deps.Findings == nil {
		return Result{}, errFindingsNotConfigured
	}
	var in struct {
		FindingID string `json:"finding_id"`
		FolderID  string `json:"folder_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	findingID, err := db.ParseUUID(in.FindingID)
	if err != nil {
		return invalid("finding_id must be a UUID from list_findings"), nil
	}
	folderID, err := db.ParseUUID(in.FolderID)
	if err != nil {
		return invalid("folder_id must be a UUID from list_folders"), nil
	}
	principal, _ := auth.FromContext(ctx)
	f, err := r.deps.Findings.Move(ctx, principal, findingID, folderID)
	switch {
	case errors.Is(err, findings.ErrNotFound):
		return toolError(codeNotFound, "finding not found"), nil
	case errors.Is(err, findings.ErrFolderNotFound):
		return toolError(codeNotFound, "folder not found"), nil
	case errors.Is(err, findings.ErrOnDashboard):
		return invalid("%s", findings.ErrOnDashboard.Error()), nil
	case errors.Is(err, folders.ErrMoveIntoPersonal):
		return invalid("a finding cannot move into My Dashboards"), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	case err != nil:
		return Result{}, err
	}
	return success("Moved finding "+f.ID, findingMovedOut{findingItemOut: r.findingItem(f), Folder: f.Folder.Title}), nil
}

func (r *Registry) listFindings(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Findings == nil {
		return Result{}, errFindingsNotConfigured
	}
	var in struct {
		DashboardID string `json:"dashboard_id"`
		FolderID    string `json:"folder_id"`
		Limit       *int   `json:"limit"`
		Cursor      string `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	var filter findings.ListFilter
	if in.DashboardID != "" {
		id, res, ok := parseDashboardID(in.DashboardID)
		if !ok {
			return res, nil
		}
		filter.DashboardID = id
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	filter.FolderID = folderID
	limit, res, ok := boundedInt(in.Limit, defaultListFindings, 1, maxListFindings, "limit")
	if !ok {
		return res, nil
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		list, next, err := r.deps.Findings.List(ctx, principal, filter, limit, in.Cursor)
		switch {
		case errors.Is(err, findings.ErrInvalidCursor):
			return invalid("cursor is not valid"), nil
		case errors.Is(err, findings.ErrPlacement):
			return invalid("dashboard_id and folder_id cannot go together"), nil
		case errors.Is(err, findings.ErrNotFound):
			return toolError(codeNotFound, notFoundMessage(filter)), nil
		case err != nil:
			return Result{}, err
		}
		out, kept, cut := r.findingPage(list, next)
		if !cut {
			return success("Listed "+plural(len(out.Findings), "finding", "findings"), out), nil
		}
		limit = max(kept, 1)
	}
}

func notFoundMessage(filter findings.ListFilter) string {
	if filter.FolderID.Valid {
		return "folder not found"
	}
	return "dashboard not found"
}

func (r *Registry) findingPage(list []findings.Finding, next string) (findingListOut, int, bool) {
	out := findingListOut{Findings: make([]findingItemOut, 0, len(list))}
	if next != "" {
		out.NextCursor = &next
	}
	budget := listBudget - jsonSize(out)
	for _, f := range list {
		out.Findings = append(out.Findings, r.findingItem(f))
	}
	kept, cut := fit(out.Findings, budget)
	if cut && len(list) > 1 {
		return out, len(kept), true
	}
	return out, len(kept), false
}

func (r *Registry) findingItem(f findings.Finding) findingItemOut {
	evidence := make([]string, len(f.Evidence))
	for i, p := range f.Evidence {
		evidence[i] = p.Title
	}
	out := findingItemOut{
		ID:                f.ID,
		Title:             f.Title,
		Markdown:          clip(f.Markdown, maxFindingOutBytes),
		MarkdownTruncated: len(f.Markdown) > maxFindingOutBytes,
		Evidence:          evidence,
		From:              f.From.UTC().Format(time.RFC3339),
		To:                f.To.UTC().Format(time.RFC3339),
		CreatedAt:         f.CreatedAt.UTC().Format(time.RFC3339),
		URL:               links.Finding(r.deps.PublicURL, f.ID),
	}
	if f.DashboardID != "" {
		out.DashboardID = &f.DashboardID
	}
	if f.FolderID != "" {
		out.FolderID = &f.FolderID
	}
	return out
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

func findingTitle(raw string) (string, bool) {
	title := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(title)
	if _, clean := revisions.CleanPrompt(title); !clean || n > findings.MaxTitleRunes || strings.ContainsAny(title, "\r\n") {
		return "", false
	}
	return title, true
}

func optionalUUID(raw string) (pgtype.UUID, error) {
	if raw == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(raw)
}
