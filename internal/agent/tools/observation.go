package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/forbidden"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameCreateObservation  = "create_observation"
	nameGetObservation     = "get_observation"
	nameListObservations   = "list_observations"
	nameAddBlock           = "add_block"
	nameUpdateBlock        = "update_block"
	nameResolveObservation = "resolve_observation"
	nameKeepBlock          = "keep_block"
	nameDiscardBlock       = "discard_block"

	codeInvalidTransition = "invalid_transition"
	codeBlockRefused      = "block_refused"

	defaultListObservations = 20
	maxListObservations     = 50
	maxCreateBlocks         = 10
	maxBlockMarkdownOut     = 2000
	maxBlockMarkdownRunes   = revisions.MaxPromptRunes
	panelCheckTitle         = "Panel block"
	requestedBlockNote      = "added as a requested block; a person who can edit keeps or discards it"

	descCreateObservation  = "Start an observation, a page of text, panel and snapshot blocks about one question, filed in a folder. Optional blocks are added in order; the first refused block stops the rest and the result names its index and the observation. Returns the id and url."
	descGetObservation     = "Read an observation: its state, summary, range, allowed actions and blocks in order, each with kind, state, version and a short view (Markdown clipped, panel title, type and datasources, snapshot id). Send a block's version when you update or keep it."
	descListObservations   = "List observations you may see, most recently changed first, only the open or resolved ones or those in one folder. Page with next_cursor."
	descAddBlock           = "Add a block to an observation: text (Markdown, no HTML), panel (a data panel whose query returns data in its range) or snapshot (freezes the data of a panel block). A person who can only comment gets a requested block that an editor keeps or discards."
	descUpdateBlock        = "Change a text or panel block: send the version you read and the new markdown, or the new panel with its range, and optionally width or position. A stale version answers version_conflict."
	descResolveObservation = "Close an observation with a one-line summary of what was found."
	descKeepBlock          = "Keep a requested block so it goes live for everyone who sees the observation. Send the block's version."
	descDiscardBlock       = "Discard a requested block. An editor, or the one who asked for it, may."

	propObservationID    = `"observation_id":{"type":"string","format":"uuid","description":"The observation, from list_observations or create_observation."}`
	propBlockID          = `"block_id":{"type":"string","format":"uuid","description":"The block, from get_observation or add_block."}`
	propBlockVersion     = `"version":{"type":"integer","minimum":1,"description":"The block's version from get_observation."}`
	propObservationFrom  = `"from":{"type":"string","format":"date-time","description":"Start of the observation's range, RFC 3339. Send with to."}`
	propObservationTo    = `"to":{"type":"string","format":"date-time","description":"End of the observation's range, RFC 3339."}`
	propBlockKind        = `"kind":{"type":"string","enum":["text","panel","snapshot"],"description":"What the block holds."}`
	propBlockMarkdown    = `"markdown":{"type":"string","minLength":1,"maxLength":4000,"description":"Markdown of a text block, without HTML."}`
	propBlockPanel       = `"panel":{"type":"object","description":"A data panel in ObserveSpec form whose queries return data; gridPos and ids are set for you."}`
	propBlockFrom        = `"from":{"type":"string","format":"date-time","description":"Start of a panel's own range, RFC 3339, with to. Default the observation's range."}`
	propBlockTo          = `"to":{"type":"string","format":"date-time","description":"End of a panel's own range, RFC 3339."}`
	propBlockSnapshotOf  = `"from_block_id":{"type":"string","format":"uuid","description":"The panel block to freeze, for a snapshot block."}`
	propBlockWidth       = `"width":{"type":"string","enum":["full","half"],"description":"Default full."}`
	propBlockPosition    = `"position":{"type":"integer","minimum":0,"description":"Index to place the block at. Default the end."}`
	blockProps           = propBlockKind + `,` + propBlockMarkdown + `,` + propBlockPanel + `,` + propBlockFrom + `,` + propBlockTo + `,` + propBlockSnapshotOf + `,` + propBlockWidth + `,` + propBlockPosition
	propObservationBlock = `"blocks":{"type":"array","maxItems":10,"items":{"type":"object","properties":{` + blockProps + `},"required":["kind"],"additionalProperties":false},"description":"Blocks to add in order, shaped as in add_block."}`

	schemaCreateObservation  = `{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":200,"description":"A short title."},"folder_id":{"type":"string","format":"uuid","description":"The folder to file it in, from list_folders. Default your My Dashboards, else General."},` + propObservationFrom + `,` + propObservationTo + `,` + propObservationBlock + `},"required":["title"],"additionalProperties":false}`
	schemaGetObservation     = `{"type":"object","properties":{` + propObservationID + `},"required":["observation_id"],"additionalProperties":false}`
	schemaListObservations   = `{"type":"object","properties":{"folder_id":{"type":"string","format":"uuid","description":"Only observations in this folder, from list_folders."},"state":{"type":"string","enum":["open","resolved"],"description":"Only observations in this state."},"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Observations per page. Default 20."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
	schemaAddBlock           = `{"type":"object","properties":{` + propObservationID + `,` + blockProps + `},"required":["observation_id","kind"],"additionalProperties":false}`
	schemaUpdateBlock        = `{"type":"object","properties":{` + propBlockID + `,` + propBlockVersion + `,` + propBlockMarkdown + `,` + propBlockPanel + `,` + propBlockFrom + `,` + propBlockTo + `,` + propBlockWidth + `,` + propBlockPosition + `},"required":["block_id","version"],"additionalProperties":false}`
	schemaResolveObservation = `{"type":"object","properties":{` + propObservationID + `,"summary":{"type":"string","minLength":1,"maxLength":500,"description":"One line on what was found."}},"required":["observation_id","summary"],"additionalProperties":false}`
	schemaKeepBlock          = `{"type":"object","properties":{` + propBlockID + `,` + propBlockVersion + `},"required":["block_id","version"],"additionalProperties":false}`
	schemaDiscardBlock       = `{"type":"object","properties":{` + propBlockID + `},"required":["block_id"],"additionalProperties":false}`
)

var errObservationsNotConfigured = errors.New("observations are not configured")

func init() { registerEntries((*Registry).observationEntries) }

func (r *Registry) observationEntries() []entry {
	return []entry{
		{Tool{Name: nameCreateObservation, Description: descCreateObservation, InputSchema: json.RawMessage(schemaCreateObservation), MCPOnly: true, MinRole: auth.RoleViewer}, r.createObservation},
		{Tool{Name: nameGetObservation, Description: descGetObservation, InputSchema: json.RawMessage(schemaGetObservation), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.getObservation},
		{Tool{Name: nameListObservations, Description: descListObservations, InputSchema: json.RawMessage(schemaListObservations), MCPOnly: true, ReadOnly: true, MinRole: auth.RoleViewer}, r.listObservations},
		{Tool{Name: nameAddBlock, Description: descAddBlock, InputSchema: json.RawMessage(schemaAddBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.addBlock},
		{Tool{Name: nameUpdateBlock, Description: descUpdateBlock, InputSchema: json.RawMessage(schemaUpdateBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.updateBlock},
		{Tool{Name: nameResolveObservation, Description: descResolveObservation, InputSchema: json.RawMessage(schemaResolveObservation), MCPOnly: true, MinRole: auth.RoleViewer}, r.resolveObservation},
		{Tool{Name: nameKeepBlock, Description: descKeepBlock, InputSchema: json.RawMessage(schemaKeepBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.keepBlock},
		{Tool{Name: nameDiscardBlock, Description: descDiscardBlock, InputSchema: json.RawMessage(schemaDiscardBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.discardBlock},
	}
}

type blockFields struct {
	Kind        string          `json:"kind"`
	Markdown    *string         `json:"markdown"`
	Panel       json.RawMessage `json:"panel"`
	From        string          `json:"from"`
	To          string          `json:"to"`
	FromBlockID string          `json:"from_block_id"`
	Width       string          `json:"width"`
	Position    *int            `json:"position"`
}

type addBlockInput struct {
	ObservationID string `json:"observation_id"`
	blockFields
}

type fieldPresence struct {
	name    string
	present bool
}

func (b blockFields) presence() []fieldPresence {
	return []fieldPresence{
		{"markdown", b.Markdown != nil},
		{"panel", len(b.Panel) > 0},
		{"from", b.From != ""},
		{"to", b.To != ""},
		{"from_block_id", b.FromBlockID != ""},
	}
}

var blockKindFields = map[observations.Kind][]string{
	observations.KindText:     {"markdown"},
	observations.KindPanel:    {"panel", "from", "to"},
	observations.KindSnapshot: {"from_block_id"},
}

type window struct{ from, to *time.Time }

func (w window) bounds(fallback window, now time.Time) (time.Time, time.Time) {
	for _, candidate := range []window{w, fallback} {
		if candidate.from != nil && candidate.to != nil {
			return *candidate.from, *candidate.to
		}
	}
	return now.Add(-defaultWindow), now
}

func parseWindow(from, to string) (window, Result, bool) {
	switch {
	case from == "" && to == "":
		return window{}, Result{}, true
	case from == "" || to == "":
		return window{}, toolError(observations.CodeInvalidRange, "from and to are given together or not at all"), false
	}
	start, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return window{}, invalid("from must be an RFC 3339 time"), false
	}
	end, err := time.Parse(time.RFC3339, to)
	if err != nil {
		return window{}, invalid("to must be an RFC 3339 time"), false
	}
	switch {
	case !start.Before(end):
		return window{}, toolError(observations.CodeInvalidRange, "from must be before to"), false
	case end.Sub(start) > observations.MaxRange:
		return window{}, toolError(observations.CodeInvalidRange, "the time range is longer than 400 days"), false
	}
	return window{&start, &end}, Result{}, true
}

type observationWrittenOut struct {
	Code          string `json:"code"`
	ObservationID string `json:"observation_id,omitempty"`
	BlockID       string `json:"block_id,omitempty"`
	State         string `json:"state"`
	Version       int    `json:"version,omitempty"`
	URL           string `json:"url,omitempty"`
	Note          string `json:"note,omitempty"`
}

type blockRefusedOut struct {
	Code          string `json:"code"`
	ObservationID string `json:"observation_id"`
	URL           string `json:"url"`
	BlockIndex    int    `json:"block_index"`
	Refused       any    `json:"refused"`
}

type observationViewOut struct {
	observations.ObservationJSON
	Blocks          []blockViewOut `json:"blocks"`
	BlocksTruncated bool           `json:"blocks_truncated,omitempty"`
}

type blockViewOut struct {
	ID                string   `json:"id"`
	Kind              string   `json:"kind"`
	State             string   `json:"state"`
	Position          int      `json:"position"`
	Width             string   `json:"width"`
	Version           int      `json:"version"`
	AllowedActions    []string `json:"allowed_actions"`
	Markdown          string   `json:"markdown,omitempty"`
	MarkdownTruncated bool     `json:"markdown_truncated,omitempty"`
	Title             string   `json:"title,omitempty"`
	Type              string   `json:"type,omitempty"`
	Datasources       []string `json:"datasources,omitempty"`
	From              string   `json:"from,omitempty"`
	To                string   `json:"to,omitempty"`
	SnapshotID        string   `json:"snapshot_id,omitempty"`
	RunID             string   `json:"run_id,omitempty"`
}

type observationListOut struct {
	Observations []observations.ObservationJSON `json:"observations"`
	NextCursor   *string                        `json:"next_cursor"`
}

func (r *Registry) observationFailure(step string, err error) (Result, error) {
	if res, ok := observationRefusal(err); ok {
		return res, nil
	}
	return datasourceFailure(step, err)
}

func observationRefusal(err error) (Result, bool) {
	var invalidInput *observations.InvalidInputError
	var conflict *observations.ConflictError
	var refusal *lifecycle.Refusal
	switch {
	case errors.Is(err, observations.ErrNotFound):
		return toolError(codeNotFound, "observation or block not found"), true
	case errors.Is(err, observations.ErrFolderNotFound):
		return toolError(codeNotFound, "folder not found"), true
	case errors.Is(err, observations.ErrInvalidCursor):
		return invalid("cursor is not valid"), true
	case errors.Is(err, observations.ErrNothingToUpdate):
		return invalid("no change given: send markdown, a panel, width or position"), true
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, forbidden.RoleMessage), true
	case errors.As(err, &conflict):
		return toolError(codeVersionConflict, fmt.Sprintf("the block changed since you read it; read it with %s and apply your change to version %d", nameGetObservation, conflict.Current.Version)), true
	case errors.As(err, &refusal):
		return toolError(codeInvalidTransition, fmt.Sprintf("%s is not allowed while the %s is %s%s", refusal.Action, refusal.Machine, refusal.State, allowedNow(refusal.Allowed))), true
	case errors.As(err, &invalidInput):
		if len(invalidInput.Issues) > 0 {
			return suggestionRejected(invalidInput.Issues), true
		}
		return toolError(invalidInput.Code, invalidInput.Message), true
	}
	return Result{}, false
}

func allowedNow(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	return "; allowed now: " + strings.Join(actions, ", ")
}

func (r *Registry) observationRun(ctx context.Context) (pgtype.UUID, error) {
	run, ok := RunFrom(ctx)
	if !ok {
		return pgtype.UUID{}, nil
	}
	id, err := optionalUUID(run.RequestID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("run request id: %w", err)
	}
	return id, nil
}

func parseObservationID(raw string) (pgtype.UUID, Result, bool) {
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("observation_id must be a UUID from list_observations or create_observation"), false
	}
	return id, Result{}, true
}

func parseBlockID(raw string) (pgtype.UUID, Result, bool) {
	id, err := db.ParseUUID(raw)
	if err != nil {
		return pgtype.UUID{}, invalid("block_id must be a UUID from get_observation or add_block"), false
	}
	return id, Result{}, true
}

func blockMarkdown(raw *string) (string, Result, bool) {
	if raw == nil {
		return "", invalid("markdown is required for a text block"), false
	}
	markdown := strings.TrimSpace(*raw)
	switch _, clean := revisions.CleanPrompt(markdown); {
	case markdown == "":
		return "", invalid("markdown must not be empty"), false
	case utf8.RuneCountInString(markdown) > maxBlockMarkdownRunes:
		return "", invalid("markdown must be at most %d characters", maxBlockMarkdownRunes), false
	case !clean:
		return "", invalid("markdown must not contain control or bidirectional override characters"), false
	case containsHTML(markdown):
		return "", toolError(string(codeHTMLNotAllowed), "the agent cannot write HTML; write Markdown without tags"), false
	}
	return markdown, Result{}, true
}

func foreignFields(b blockFields, kind observations.Kind) (Result, bool) {
	allowed := blockKindFields[kind]
	for _, f := range b.presence() {
		if f.present && !slices.Contains(allowed, f.name) {
			return invalid("%s does not go with a %s block; a %s block takes %s", f.name, kind, kind, strings.Join(allowed, ", ")), false
		}
	}
	return Result{}, true
}

func (r *Registry) checkPanel(ctx context.Context, principal auth.Principal, panel json.RawMessage, own, fallback window) (Result, bool, error) {
	from, to := own.bounds(fallback, time.Now())
	_, res, ok, err := r.evidenceDashboard(ctx, principal, findings.SaveInput{Title: panelCheckTitle, From: from, To: to}, []json.RawMessage{panel}, time.Now())
	return res, ok, err
}

func checkKind(kind observations.Kind) (Result, bool) {
	if kind == "" {
		return invalid("kind is required: text, panel or snapshot"), false
	}
	if _, known := blockKindFields[kind]; !known {
		return invalid("kind must be text, panel or snapshot"), false
	}
	return Result{}, true
}

func (r *Registry) blockInput(ctx context.Context, principal auth.Principal, b blockFields, fallback window, runID pgtype.UUID) (observations.BlockInput, Result, bool, error) {
	in := observations.BlockInput{Kind: observations.Kind(b.Kind), Width: observations.Width(b.Width), Position: b.Position, RunID: runID}
	if res, ok := checkKind(in.Kind); !ok {
		return in, res, false, nil
	}
	if res, ok := foreignFields(b, in.Kind); !ok {
		return in, res, false, nil
	}
	var err error
	switch in.Kind {
	case observations.KindText:
		markdown, res, ok := blockMarkdown(b.Markdown)
		if !ok {
			return in, res, false, nil
		}
		in.Body, err = json.Marshal(observations.TextBody{Markdown: markdown})
	case observations.KindPanel:
		if len(b.Panel) == 0 {
			return in, invalid("panel is required for a panel block"), false, nil
		}
		own, res, ok := parseWindow(b.From, b.To)
		if !ok {
			return in, res, false, nil
		}
		if res, ok, err := r.checkPanel(ctx, principal, b.Panel, own, fallback); err != nil || !ok {
			return in, res, false, err
		}
		in.Body, err = json.Marshal(observations.PanelBody{Panel: b.Panel, From: own.from, To: own.to})
	case observations.KindSnapshot:
		if in.FromBlockID, err = db.ParseUUID(b.FromBlockID); err != nil {
			return in, invalid("from_block_id must be the UUID of a panel block from get_observation"), false, nil
		}
	}
	if err != nil {
		return in, Result{}, false, fmt.Errorf("encode block body: %w", err)
	}
	return in, Result{}, true, nil
}

func (r *Registry) addedBlock(b observations.Block) Result {
	out := observationWrittenOut{Code: "ok", ObservationID: b.ObservationID, BlockID: b.ID, State: string(b.State), Version: b.Version, URL: r.deps.Observations.URL(b.ObservationID)}
	if b.State == observations.BlockRequested {
		out.Note = requestedBlockNote
	}
	return success("Added a "+string(b.Kind)+" block to "+b.ObservationID, out)
}

func (r *Registry) addBlockTo(ctx context.Context, principal auth.Principal, observationID pgtype.UUID, b blockFields, fallback window, runID pgtype.UUID) (observations.Block, Result, bool, error) {
	in, res, ok, err := r.blockInput(ctx, principal, b, fallback, runID)
	if err != nil || !ok {
		return observations.Block{}, res, false, err
	}
	block, err := r.deps.Observations.AddBlock(ctx, principal, observationID, in)
	if err != nil {
		res, err := r.observationFailure("Added a block", err)
		return observations.Block{}, res, false, err
	}
	return block, Result{}, true, nil
}

func (r *Registry) createObservation(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		Title    string        `json:"title"`
		FolderID string        `json:"folder_id"`
		From     string        `json:"from"`
		To       string        `json:"to"`
		Blocks   []blockFields `json:"blocks"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if len(in.Blocks) > maxCreateBlocks {
		return invalid("blocks takes at most %d items", maxCreateBlocks), nil
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	rng, res, ok := parseWindow(in.From, in.To)
	if !ok {
		return res, nil
	}
	runID, err := r.observationRun(ctx)
	if err != nil {
		return Result{}, err
	}
	principal, _ := auth.FromContext(ctx)
	o, err := r.deps.Observations.Create(ctx, principal, observations.CreateInput{FolderID: folderID, Title: in.Title, From: rng.from, To: rng.to, RunID: runID})
	if err != nil {
		return r.observationFailure("Created an observation", err)
	}
	id, err := db.ParseUUID(o.ID)
	if err != nil {
		return Result{}, fmt.Errorf("created observation id: %w", err)
	}
	url := r.deps.Observations.URL(o.ID)
	for i, b := range in.Blocks {
		_, res, ok, err := r.addBlockTo(ctx, principal, id, b, rng, runID)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			step := fmt.Sprintf("Created observation %s, but block %d was refused", o.ID, i)
			return failure(step, blockRefusedOut{Code: codeBlockRefused, ObservationID: o.ID, URL: url, BlockIndex: i, Refused: res.Content}), nil
		}
	}
	return success("Created observation "+o.ID, observationWrittenOut{Code: "ok", ObservationID: o.ID, State: string(o.State), URL: url}), nil
}

func (r *Registry) addBlock(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in addBlockInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	if res, ok := checkKind(observations.Kind(in.Kind)); !ok {
		return res, nil
	}
	runID, err := r.observationRun(ctx)
	if err != nil {
		return Result{}, err
	}
	principal, _ := auth.FromContext(ctx)
	o, err := r.deps.Observations.Get(ctx, principal, id)
	if err != nil {
		return r.observationFailure("Read an observation", err)
	}
	block, res, ok, err := r.addBlockTo(ctx, principal, id, in.blockFields, window{o.From, o.To}, runID)
	if err != nil || !ok {
		return res, err
	}
	return r.addedBlock(block), nil
}

func (r *Registry) updateBlock(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in blockUpdateInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseBlockID(in.BlockID)
	if !ok {
		return res, nil
	}
	if in.Version == nil {
		return invalid("version is required; read it with %s", nameGetObservation), nil
	}
	runID, err := r.observationRun(ctx)
	if err != nil {
		return Result{}, err
	}
	principal, _ := auth.FromContext(ctx)
	change := observations.BlockChange{Version: *in.Version, Position: in.Position, RunID: runID}
	if in.Width != "" {
		width := observations.Width(in.Width)
		change.Width = &width
	}
	if res, ok, err := r.blockBody(ctx, principal, id, in, &change); err != nil || !ok {
		return res, err
	}
	block, err := r.deps.Observations.UpdateBlock(ctx, principal, id, change)
	if err != nil {
		return r.observationFailure("Updated a block", err)
	}
	out := observationWrittenOut{Code: "ok", ObservationID: block.ObservationID, BlockID: block.ID, State: string(block.State), Version: block.Version, URL: r.deps.Observations.URL(block.ObservationID)}
	if block.State == observations.BlockRequested {
		out.Note = requestedBlockNote
	}
	return success("Updated block "+block.ID, out), nil
}

type blockUpdateInput struct {
	BlockID  string          `json:"block_id"`
	Version  *int            `json:"version"`
	Markdown *string         `json:"markdown"`
	Panel    json.RawMessage `json:"panel"`
	From     string          `json:"from"`
	To       string          `json:"to"`
	Width    string          `json:"width"`
	Position *int            `json:"position"`
}

func (r *Registry) blockBody(ctx context.Context, principal auth.Principal, id pgtype.UUID, in blockUpdateInput, change *observations.BlockChange) (Result, bool, error) {
	switch {
	case in.Markdown != nil && len(in.Panel) > 0:
		return invalid("send markdown for a text block or panel for a panel block, not both"), false, nil
	case in.Markdown != nil:
		if in.From != "" || in.To != "" {
			return invalid("from and to go with panel"), false, nil
		}
		markdown, res, ok := blockMarkdown(in.Markdown)
		if !ok {
			return res, false, nil
		}
		body, err := json.Marshal(observations.TextBody{Markdown: markdown})
		if err != nil {
			return Result{}, false, fmt.Errorf("encode block body: %w", err)
		}
		change.Body = body
	case len(in.Panel) > 0:
		body, res, ok, err := r.panelChange(ctx, principal, id, in.Panel, in.From, in.To)
		if err != nil || !ok {
			return res, false, err
		}
		change.Body = body
	case in.From != "" || in.To != "":
		return invalid("from and to go with panel"), false, nil
	}
	return Result{}, true, nil
}

func (r *Registry) panelChange(ctx context.Context, principal auth.Principal, blockID pgtype.UUID, panel json.RawMessage, from, to string) (json.RawMessage, Result, bool, error) {
	own, res, ok := parseWindow(from, to)
	if !ok {
		return nil, res, false, nil
	}
	_, shown, end, err := r.deps.Observations.PanelDashboard(ctx, principal, blockID)
	if errors.Is(err, observations.ErrNotFound) {
		return nil, toolError(codeNotFound, "panel block not found"), false, nil
	}
	if err != nil {
		res, err := r.observationFailure("Read a block", err)
		return nil, res, false, err
	}
	if res, ok, err := r.checkPanel(ctx, principal, panel, own, window{&shown, &end}); err != nil || !ok {
		return nil, res, false, err
	}
	body, err := json.Marshal(observations.PanelBody{Panel: panel, From: own.from, To: own.to})
	if err != nil {
		return nil, Result{}, false, fmt.Errorf("encode block body: %w", err)
	}
	return body, Result{}, true, nil
}

func (r *Registry) resolveObservation(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
		Summary       string `json:"summary"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	o, err := r.deps.Observations.Resolve(ctx, principal, id, in.Summary)
	if err != nil {
		return r.observationFailure("Resolved an observation", err)
	}
	return success("Resolved observation "+o.ID, observationWrittenOut{Code: "ok", ObservationID: o.ID, State: string(o.State), URL: r.deps.Observations.URL(o.ID)}), nil
}

func (r *Registry) keepBlock(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		BlockID string `json:"block_id"`
		Version *int   `json:"version"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseBlockID(in.BlockID)
	if !ok {
		return res, nil
	}
	if in.Version == nil {
		return invalid("version is required; read it with %s", nameGetObservation), nil
	}
	runID, err := r.observationRun(ctx)
	if err != nil {
		return Result{}, err
	}
	principal, _ := auth.FromContext(ctx)
	block, err := r.deps.Observations.KeepBlock(ctx, principal, id, *in.Version, runID)
	if err != nil {
		return r.observationFailure("Kept a block", err)
	}
	return success("Kept block "+block.ID, observationWrittenOut{Code: "ok", ObservationID: block.ObservationID, BlockID: block.ID, State: string(block.State), Version: block.Version, URL: r.deps.Observations.URL(block.ObservationID)}), nil
}

func (r *Registry) discardBlock(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		BlockID string `json:"block_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseBlockID(in.BlockID)
	if !ok {
		return res, nil
	}
	runID, err := r.observationRun(ctx)
	if err != nil {
		return Result{}, err
	}
	principal, _ := auth.FromContext(ctx)
	if err := r.deps.Observations.DiscardBlock(ctx, principal, id, runID); err != nil {
		return r.observationFailure("Discarded a block", err)
	}
	return success("Discarded block "+id.String(), observationWrittenOut{Code: "ok", BlockID: id.String(), State: "discarded"}), nil
}

func (r *Registry) getObservation(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	o, err := r.deps.Observations.Get(ctx, principal, id)
	if err != nil {
		return r.observationFailure("Read an observation", err)
	}
	blocks := o.Blocks
	o.Blocks = nil
	out := observationViewOut{ObservationJSON: observations.ObservationOf(o, r.deps.Observations.URL(o.ID)), Blocks: make([]blockViewOut, len(blocks))}
	for i, b := range blocks {
		out.Blocks[i] = blockView(b)
	}
	budget := listBudget - jsonSize(observationViewOut{ObservationJSON: out.ObservationJSON, Blocks: []blockViewOut{}})
	kept, cut := fit(out.Blocks, budget)
	out.Blocks, out.BlocksTruncated = kept, cut
	return success("Read observation "+o.ID+": "+plural(len(kept), "block", "blocks"), out), nil
}

func blockView(b observations.Block) blockViewOut {
	base := observations.BlockOf(b)
	out := blockViewOut{
		ID: base.ID, Kind: base.Kind, State: base.State, Position: base.Position, Width: base.Width, Version: base.Version,
		AllowedActions: base.AllowedActions, SnapshotID: b.SnapshotID, RunID: b.RunID,
	}
	switch b.Kind {
	case observations.KindText:
		var body observations.TextBody
		if json.Unmarshal(b.Body, &body) == nil {
			out.Markdown, out.MarkdownTruncated = clipRunes(body.Markdown, maxBlockMarkdownOut)
		}
	case observations.KindPanel:
		var body observations.PanelBody
		if json.Unmarshal(b.Body, &body) != nil {
			break
		}
		var panel spec.Panel
		if json.Unmarshal(body.Panel, &panel) == nil {
			out.Title, out.Type, out.Datasources = panel.Title, string(panel.Type), panelDatasources(panel)
		}
		if body.From != nil && body.To != nil {
			out.From, out.To = body.From.UTC().Format(time.RFC3339), body.To.UTC().Format(time.RFC3339)
		}
	}
	return out
}

func panelDatasources(p spec.Panel) []string {
	var uids []string
	for _, uid := range append([]string{p.Datasource}, queryDatasources(p.Queries)...) {
		if uid != "" && !slices.Contains(uids, uid) {
			uids = append(uids, uid)
		}
	}
	return uids
}

func queryDatasources(queries []spec.Query) []string {
	uids := make([]string, len(queries))
	for i, q := range queries {
		uids[i] = q.Datasource
	}
	return uids
}

func clipRunes(s string, limit int) (string, bool) {
	if utf8.RuneCountInString(s) <= limit {
		return s, false
	}
	return string([]rune(s)[:limit]), true
}

func (r *Registry) listObservations(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		FolderID string `json:"folder_id"`
		State    string `json:"state"`
		Limit    *int   `json:"limit"`
		Cursor   string `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return res, nil
	}
	filter := observations.ListFilter{FolderID: folderID, State: observations.State(in.State)}
	if filter.State != "" && filter.State != observations.StateOpen && filter.State != observations.StateResolved {
		return invalid("state must be open or resolved"), nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultListObservations, 1, maxListObservations, "limit")
	if !ok {
		return res, nil
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		list, next, err := r.deps.Observations.List(ctx, principal, filter, limit, in.Cursor)
		if err != nil {
			return r.observationFailure("Listed observations", err)
		}
		out := observationListOut{Observations: make([]observations.ObservationJSON, len(list))}
		for i, o := range list {
			out.Observations[i] = observations.ObservationOf(o, r.deps.Observations.URL(o.ID))
		}
		if next != "" {
			out.NextCursor = &next
		}
		kept, cut := fit(out.Observations, listBudget-jsonSize(observationListOut{Observations: []observations.ObservationJSON{}, NextCursor: out.NextCursor}))
		if !cut || len(list) <= 1 {
			return success("Listed "+plural(len(out.Observations), "observation", "observations"), out), nil
		}
		limit = max(len(kept), 1)
	}
}
