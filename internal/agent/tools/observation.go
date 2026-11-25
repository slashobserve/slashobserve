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
	nameArrangeBlocks      = "arrange_blocks"
	nameResolveObservation = "resolve_observation"
	nameKeepBlock          = "keep_block"
	nameDiscardBlock       = "discard_block"

	codeInvalidTransition = "invalid_transition"
	codeBlockRefused      = "block_refused"
	blockRefusedMessage   = "a block was refused and no observation was created; the reason is in refused, so fix it and send the call again"

	defaultListObservations = 20
	maxListObservations     = 50
	maxCreateBlocks         = 10
	maxBlockMarkdownOut     = 2000
	maxBlockMarkdownRunes   = revisions.MaxPromptRunes
	panelCheckTitle         = "Panel block"
	requestedBlockNote      = "added as a requested block; a person who can edit keeps or discards it"

	descCreateObservation  = "Start an observation, a page of text, panel and snapshot blocks about one question, filed in a folder. Optional blocks are created with it, in order: a refused block creates nothing and the result names its index. Returns the id and url."
	descGetObservation     = "Read an observation: its state, summary (the last one given, kept when it is reopened), range, allowed actions and blocks in order, each with kind, state, version and a short view (Markdown clipped, panel title, type and datasources, snapshot id) and, for a panel block, its full panel, so you can read what you added before you revise it."
	descListObservations   = "List observations you may see, most recently changed first, only the open or resolved ones or those in one folder. Page with next_cursor."
	descAddBlock           = "Add a block to an observation: text (Markdown, no HTML), panel (a data panel whose query returns data in its range) or snapshot (freezes the data of a panel block). A panel that uses $variables needs their values in variables, or the block is refused as variable_missing. A person who can only comment gets a requested block that an editor keeps or discards."
	descUpdateBlock        = "Change a text or panel block: send the version you read and the new markdown, or the new panel with its range, and optionally width, height or position. follow_page: true drops a panel's own range so it shows the observation's. A stale version answers version_conflict, and a width, height or position change raises the version too, so to lay out a page use arrange_blocks."
	descArrangeBlocks      = "Set the order of an observation's page and the width and height of its blocks in one call: list block_ids in the order the page should show them, each with an optional width (third, half, two_thirds or full) and height in pixels (120 to 960, null for the default). A block you leave out keeps its place after the block it followed. It needs no versions and never makes a person's edit of a block stale; the last arrangement wins."
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
	propBlockVariables   = `"variables":{"type":"object","additionalProperties":{"type":"array","items":{"type":"string"}}}`
	propBlockFrom        = `"from":{"type":"string","format":"date-time","description":"Start of a panel's own range, RFC 3339, with to. Default the observation's range."}`
	propBlockTo          = `"to":{"type":"string","format":"date-time","description":"End of a panel's own range, RFC 3339."}`
	propBlockSnapshotOf  = `"from_block_id":{"type":"string","format":"uuid","description":"The panel block to freeze, for a snapshot block."}`
	propBlockWidth       = `"width":{"type":"string","enum":["third","half","two_thirds","full"],"description":"Default full."}`
	propBlockHeight      = `"height":{"type":"integer","minimum":120,"maximum":960,"description":"Pixels, for a panel or snapshot block. Default by width."}`
	propUpdateHeight     = `"height":{"type":["integer","null"],"minimum":120,"maximum":960,"description":"Pixels, for a panel or snapshot block; null for the default."}`
	propArrangedBlocks   = `"blocks":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"object","properties":{` + propBlockID + `,` + propBlockWidth + `,` + propUpdateHeight + `},"required":["block_id"],"additionalProperties":false},"description":"The blocks in the order the page should show them, each at most once. A width or height you leave out is kept."}`
	propBlockPosition    = `"position":{"type":"integer","minimum":0,"description":"Index to place the block at. Default the end."}`
	propBlockFollowPage  = `"follow_page":{"type":"boolean","description":"True drops a panel block's own from and to, so it shows the observation's range. Send without panel, markdown, from or to."}`
	blockProps           = propBlockKind + `,` + propBlockMarkdown + `,` + propBlockPanel + `,` + propBlockVariables + `,` + propBlockFrom + `,` + propBlockTo + `,` + propBlockSnapshotOf + `,` + propBlockWidth + `,` + propBlockPosition
	propObservationBlock = `"blocks":{"type":"array","maxItems":10,"items":{"type":"object","properties":{` + blockProps + `},"required":["kind"],"additionalProperties":false},"description":"Blocks to add in order, shaped as in add_block but without height."}`

	schemaCreateObservation  = `{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":200,"description":"A short title."},"folder_id":{"type":"string","format":"uuid","description":"The folder to file it in, from list_folders. Default your My Dashboards, else General."},` + propObservationFrom + `,` + propObservationTo + `,` + propObservationBlock + `},"required":["title"],"additionalProperties":false}`
	schemaGetObservation     = `{"type":"object","properties":{"observation_id":{"type":"string","format":"uuid","description":"The observation, from list_observations."}},"required":["observation_id"],"additionalProperties":false}`
	schemaListObservations   = `{"type":"object","properties":{{observations_folder_prop}{observations_restricted_prop}"state":{"type":"string","enum":["open","resolved"],"description":"Only observations in this state."},"title":{"type":"string","maxLength":200,"description":"Only observations whose title contains this text, ignoring case."},"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Observations per page. Default 20."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
	schemaAddBlock           = `{"type":"object","properties":{` + propObservationID + `,` + blockProps + `,` + propBlockHeight + `},"required":["observation_id","kind"],"additionalProperties":false}`
	schemaUpdateBlock        = `{"type":"object","properties":{` + propBlockID + `,` + propBlockVersion + `,` + propBlockMarkdown + `,` + propBlockPanel + `,` + propBlockVariables + `,` + propBlockFrom + `,` + propBlockTo + `,` + propBlockWidth + `,` + propUpdateHeight + `,` + propBlockPosition + `,` + propBlockFollowPage + `},"required":["block_id","version"],"additionalProperties":false}`
	schemaArrangeBlocks      = `{"type":"object","properties":{` + propObservationID + `,` + propArrangedBlocks + `},"required":["observation_id","blocks"],"additionalProperties":false}`
	schemaResolveObservation = `{"type":"object","properties":{` + propObservationID + `,"summary":{"type":"string","minLength":1,"maxLength":500,"description":"One line on what was found."}},"required":["observation_id","summary"],"additionalProperties":false}`
	schemaKeepBlock          = `{"type":"object","properties":{` + propBlockID + `,` + propBlockVersion + `},"required":["block_id","version"],"additionalProperties":false}`
	schemaDiscardBlock       = `{"type":"object","properties":{` + propBlockID + `},"required":["block_id"],"additionalProperties":false}`
)

var errObservationsNotConfigured = errors.New("observations are not configured")

func init() { registerEntries((*Registry).observationEntries) }

func (r *Registry) observationEntries() []entry {
	return []entry{
		{Tool{Name: nameCreateObservation, Description: descCreateObservation, InputSchema: json.RawMessage(schemaCreateObservation), MCPOnly: true, MinRole: auth.RoleViewer}, r.createObservation},
		{Tool{Name: nameGetObservation, Description: descGetObservation, InputSchema: json.RawMessage(schemaGetObservation), ReadOnly: true, MinRole: auth.RoleViewer}, r.getObservation},
		{Tool{Name: nameListObservations, Description: descListObservations, InputSchema: json.RawMessage(schemaListObservations), ReadOnly: true, MinRole: auth.RoleViewer}, r.listObservations},
		{Tool{Name: nameAddBlock, Description: descAddBlock, InputSchema: json.RawMessage(schemaAddBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.addBlock},
		{Tool{Name: nameUpdateBlock, Description: descUpdateBlock, InputSchema: json.RawMessage(schemaUpdateBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.updateBlock},
		{Tool{Name: nameArrangeBlocks, Description: descArrangeBlocks, InputSchema: json.RawMessage(schemaArrangeBlocks), MCPOnly: true, MinRole: auth.RoleViewer}, r.arrangeBlocks},
		{Tool{Name: nameResolveObservation, Description: descResolveObservation, InputSchema: json.RawMessage(schemaResolveObservation), MCPOnly: true, MinRole: auth.RoleViewer}, r.resolveObservation},
		{Tool{Name: nameKeepBlock, Description: descKeepBlock, InputSchema: json.RawMessage(schemaKeepBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.keepBlock},
		{Tool{Name: nameDiscardBlock, Description: descDiscardBlock, InputSchema: json.RawMessage(schemaDiscardBlock), MCPOnly: true, MinRole: auth.RoleViewer}, r.discardBlock},
	}
}

type blockFields struct {
	Kind        string              `json:"kind"`
	Markdown    *string             `json:"markdown"`
	Panel       json.RawMessage     `json:"panel"`
	Variables   map[string][]string `json:"variables"`
	From        string              `json:"from"`
	To          string              `json:"to"`
	FromBlockID string              `json:"from_block_id"`
	Width       string              `json:"width"`
	Height      *int                `json:"height"`
	Position    *int                `json:"position"`
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
		{"variables", len(b.Variables) > 0},
		{"from", b.From != ""},
		{"to", b.To != ""},
		{"from_block_id", b.FromBlockID != ""},
	}
}

var blockKindFields = map[observations.Kind][]string{
	observations.KindText:     {"markdown"},
	observations.KindPanel:    {"panel", "variables", "from", "to"},
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
	ObservationID string `json:"observation_id,omitempty"`
	BlockID       string `json:"block_id,omitempty"`
	State         string `json:"state"`
	Version       int    `json:"version,omitempty"`
	URL           string `json:"url,omitempty"`
	Note          string `json:"note,omitempty"`
}

type blockRefusedOut struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	BlockIndex int    `json:"block_index"`
	Refused    any    `json:"refused"`
}

type observationViewOut struct {
	observations.ObservationJSON
	Blocks          []blockViewOut `json:"blocks"`
	BlocksTruncated bool           `json:"blocks_truncated,omitempty"`
}

type blockViewOut struct {
	ID                string              `json:"id"`
	Kind              string              `json:"kind"`
	State             string              `json:"state"`
	Position          int                 `json:"position"`
	Width             string              `json:"width"`
	Height            *int                `json:"height"`
	Version           int                 `json:"version"`
	AllowedActions    []string            `json:"allowed_actions"`
	Markdown          string              `json:"markdown,omitempty"`
	MarkdownTruncated bool                `json:"markdown_truncated,omitempty"`
	Title             string              `json:"title,omitempty"`
	Type              string              `json:"type,omitempty"`
	Datasources       []string            `json:"datasources,omitempty"`
	Panel             json.RawMessage     `json:"panel,omitempty"`
	Variables         map[string][]string `json:"variables,omitempty"`
	From              string              `json:"from,omitempty"`
	To                string              `json:"to,omitempty"`
	SnapshotID        string              `json:"snapshot_id,omitempty"`
	RunID             string              `json:"run_id,omitempty"`
}

type observationListOut struct {
	Observations []observations.ObservationJSON `json:"observations"`
	NextCursor   *string                        `json:"next_cursor"`
	LeftOut      int                            `json:"left_out,omitempty"`
	LeftOutWhy   string                         `json:"left_out_note,omitempty"`
}

func (r *Registry) observationFailure(ctx context.Context, step string, err error) (Result, error) {
	if res, ok := observationRefusal(ctx, err); ok {
		return res, nil
	}
	return datasourceFailure(step, err)
}

func observationRefusal(ctx context.Context, err error) (Result, bool) {
	if res, ok := restrictedRefusal(ctx, err); ok {
		return res, true
	}
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
		return invalid("no change given: send markdown, a panel, width, height or position"), true
	case errors.Is(err, folders.ErrForbidden):
		return forbiddenBy(err, forbidden.RoleMessage), true
	case errors.As(err, &conflict):
		return toolError(codeVersionConflict, fmt.Sprintf("the block changed since you read it; read it with %s and apply your change to version %d", nameGetObservation, conflict.Current.Version)), true
	case errors.As(err, &refusal):
		return toolError(codeInvalidTransition, fmt.Sprintf("%s is not allowed while the %s is %s%s", refusal.Action, refusal.Machine, refusal.State, allowedNow(refusal.Allowed))), true
	case errors.As(err, &invalidInput):
		if len(invalidInput.Issues) > 0 && invalidInput.Code != observations.CodeVariableMissing {
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
	switch _, textErr := revisions.CheckPrompt("markdown", markdown); {
	case markdown == "":
		return "", invalid("markdown must not be empty"), false
	case utf8.RuneCountInString(markdown) > maxBlockMarkdownRunes:
		return "", invalid("markdown must be at most %d characters", maxBlockMarkdownRunes), false
	case textErr != nil:
		return "", invalid("%s", textErr), false
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

func (r *Registry) checkPanel(ctx context.Context, principal auth.Principal, panel json.RawMessage, values map[string][]string, own, fallback window) (Result, bool, error) {
	from, to := own.bounds(fallback, Now(ctx))
	variables, err := observations.ValueVariables(values)
	if err != nil {
		res, _ := observationRefusal(ctx, err)
		return res, false, nil
	}
	_, res, ok, err := r.evidenceDashboardWith(ctx, principal, findings.SaveInput{Title: panelCheckTitle, From: from, To: to}, []json.RawMessage{panel}, variables, true, Now(ctx))
	return res, ok, err
}

func (r *Registry) CheckPanel(ctx context.Context, by auth.Principal, body observations.PanelBody, fallbackFrom, fallbackTo *time.Time) error {
	res, ok, err := r.checkPanel(ctx, by, body.Panel, body.VariableValues, window{body.From, body.To}, window{fallbackFrom, fallbackTo})
	if err != nil || ok {
		return err
	}
	return refusalOf(res)
}

func refusalOf(res Result) error {
	switch content := res.Content.(type) {
	case invalidSpecOut:
		return &observations.InvalidInputError{Code: content.Code, Message: "the panel is invalid", Issues: content.Issues}
	case patchRejectedOut:
		return &observations.InvalidInputError{Code: content.Code, Message: "the panel is invalid", Issues: content.Issues}
	case errorBody:
		return &observations.InvalidInputError{Code: content.Code, Message: content.Message}
	}
	return errors.New("a panel check failed in a way a caller cannot read")
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
	in := observations.BlockInput{Kind: observations.Kind(b.Kind), Width: observations.Width(b.Width), Height: b.Height, Position: b.Position, RunID: runID}
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
		if res, ok, err := r.checkPanel(ctx, principal, b.Panel, b.Variables, own, fallback); err != nil || !ok {
			return in, res, false, err
		}
		in.Body, err = json.Marshal(observations.PanelBody{Panel: b.Panel, VariableValues: b.Variables, From: own.from, To: own.to})
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
	out := observationWrittenOut{ObservationID: b.ObservationID, BlockID: b.ID, State: string(b.State), Version: b.Version, URL: r.deps.Observations.URL(b.ObservationID)}
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
		res, err := r.observationFailure(ctx, "Added a block", err)
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
	blocks := make([]observations.BlockInput, len(in.Blocks))
	for i, b := range in.Blocks {
		block, res, ok, err := r.blockInput(ctx, principal, b, rng, runID)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			return blockRefused(i, res), nil
		}
		blocks[i] = block
	}
	o, err := r.deps.Observations.Create(ctx, principal, observations.CreateInput{FolderID: folderID, Title: in.Title, From: rng.from, To: rng.to, Blocks: blocks, RunID: runID})
	var refused *observations.BlockError
	if errors.As(err, &refused) {
		res, err := r.observationFailure(ctx, "Created an observation", refused.Err)
		if err != nil {
			return Result{}, err
		}
		return blockRefused(refused.Index, res), nil
	}
	if err != nil {
		return r.observationFailure(ctx, "Created an observation", err)
	}
	return success("Created observation "+o.ID, observationWrittenOut{ObservationID: o.ID, State: string(o.State), URL: r.deps.Observations.URL(o.ID)}), nil
}

func blockRefused(index int, res Result) Result {
	step := fmt.Sprintf("Nothing was created: block %d was refused", index)
	return failure(step, blockRefusedOut{Code: codeBlockRefused, Message: blockRefusedMessage, BlockIndex: index, Refused: res.Content})
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
		return r.observationFailure(ctx, "Read an observation", err)
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
	change := observations.BlockChange{Version: *in.Version, Height: in.Height, Position: in.Position, FollowPage: in.FollowPage, RunID: runID}
	if in.Width != "" {
		width := observations.Width(in.Width)
		change.Width = &width
	}
	if res, ok, err := r.blockBody(ctx, principal, id, in, &change); err != nil || !ok {
		return res, err
	}
	block, err := r.deps.Observations.UpdateBlock(ctx, principal, id, change)
	if err != nil {
		return r.observationFailure(ctx, "Updated a block", err)
	}
	out := observationWrittenOut{ObservationID: block.ObservationID, BlockID: block.ID, State: string(block.State), Version: block.Version, URL: r.deps.Observations.URL(block.ObservationID)}
	if block.State == observations.BlockRequested {
		out.Note = requestedBlockNote
	}
	return success("Updated block "+block.ID, out), nil
}

type blockUpdateInput struct {
	BlockID    string                    `json:"block_id"`
	Version    *int                      `json:"version"`
	Markdown   *string                   `json:"markdown"`
	Panel      json.RawMessage           `json:"panel"`
	Variables  map[string][]string       `json:"variables"`
	From       string                    `json:"from"`
	To         string                    `json:"to"`
	Width      string                    `json:"width"`
	Height     observations.HeightChange `json:"height"`
	Position   *int                      `json:"position"`
	FollowPage bool                      `json:"follow_page"`
}

func (in blockUpdateInput) carriesBody() bool {
	return in.Markdown != nil || len(in.Panel) > 0 || in.From != "" || in.To != ""
}

func (r *Registry) blockBody(ctx context.Context, principal auth.Principal, id pgtype.UUID, in blockUpdateInput, change *observations.BlockChange) (Result, bool, error) {
	if in.FollowPage && in.carriesBody() {
		return invalid("follow_page goes without panel, markdown, from or to"), false, nil
	}
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
		body, res, ok, err := r.panelChange(ctx, principal, id, in.Panel, in.Variables, in.From, in.To)
		if err != nil || !ok {
			return res, false, err
		}
		change.Body = body
	case in.From != "" || in.To != "":
		return invalid("from and to go with panel"), false, nil
	}
	return Result{}, true, nil
}

func (r *Registry) panelChange(ctx context.Context, principal auth.Principal, blockID pgtype.UUID, panel json.RawMessage, values map[string][]string, from, to string) (json.RawMessage, Result, bool, error) {
	own, res, ok := parseWindow(from, to)
	if !ok {
		return nil, res, false, nil
	}
	_, shown, end, err := r.deps.Observations.PanelDashboard(ctx, principal, blockID)
	if errors.Is(err, observations.ErrNotFound) {
		return nil, toolError(codeNotFound, "panel block not found"), false, nil
	}
	if err != nil {
		res, err := r.observationFailure(ctx, "Read a block", err)
		return nil, res, false, err
	}
	if res, ok, err := r.checkPanel(ctx, principal, panel, values, own, window{&shown, &end}); err != nil || !ok {
		return nil, res, false, err
	}
	body, err := json.Marshal(observations.PanelBody{Panel: panel, VariableValues: values, From: own.from, To: own.to})
	if err != nil {
		return nil, Result{}, false, fmt.Errorf("encode block body: %w", err)
	}
	return body, Result{}, true, nil
}

type arrangeBlocksInput struct {
	ObservationID string `json:"observation_id"`
	Blocks        []struct {
		BlockID string                    `json:"block_id"`
		Width   string                    `json:"width"`
		Height  observations.HeightChange `json:"height"`
	} `json:"blocks"`
}

type arrangedBlockOut struct {
	BlockID  string `json:"block_id"`
	Position int    `json:"position"`
	Width    string `json:"width"`
	Height   *int   `json:"height"`
}

type arrangedOut struct {
	ObservationID string             `json:"observation_id"`
	Blocks        []arrangedBlockOut `json:"blocks"`
	URL           string             `json:"url"`
}

func (r *Registry) arrangeBlocks(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in arrangeBlocksInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	if len(in.Blocks) == 0 {
		return invalid("blocks must list at least one block, in the order the page should show them"), nil
	}
	items := make([]observations.BlockLayout, len(in.Blocks))
	for i, b := range in.Blocks {
		blockID, res, ok := parseBlockID(b.BlockID)
		if !ok {
			return res, nil
		}
		items[i] = observations.BlockLayout{ID: blockID, Height: b.Height}
		if b.Width != "" {
			width := observations.Width(b.Width)
			items[i].Width = &width
		}
	}
	runID, err := r.observationRun(ctx)
	if err != nil {
		return Result{}, err
	}
	principal, _ := auth.FromContext(ctx)
	arranged, err := r.deps.Observations.Arrange(ctx, principal, id, items, runID)
	if err != nil {
		return r.observationFailure(ctx, "Arranged an observation", err)
	}
	out := arrangedOut{ObservationID: id.String(), Blocks: make([]arrangedBlockOut, len(arranged)), URL: r.deps.Observations.URL(id.String())}
	for i, b := range arranged {
		out.Blocks[i] = arrangedBlockOut{BlockID: b.ID, Position: b.Position, Width: string(b.Width), Height: b.Height}
	}
	return success("Arranged observation "+out.ObservationID+": "+plural(len(items), "block", "blocks"), out), nil
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
		return r.observationFailure(ctx, "Resolved an observation", err)
	}
	return success("Resolved observation "+o.ID, observationWrittenOut{ObservationID: o.ID, State: string(o.State), URL: r.deps.Observations.URL(o.ID)}), nil
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
		return r.observationFailure(ctx, "Kept a block", err)
	}
	return success("Kept block "+block.ID, observationWrittenOut{ObservationID: block.ObservationID, BlockID: block.ID, State: string(block.State), Version: block.Version, URL: r.deps.Observations.URL(block.ObservationID)}), nil
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
		return r.observationFailure(ctx, "Discarded a block", err)
	}
	return success("Discarded block "+id.String(), observationWrittenOut{BlockID: id.String(), State: "discarded"}), nil
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
		return r.observationFailure(ctx, "Read an observation", err)
	}
	readContentIn(ctx, o.ID, o.Folder)
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
		ID: base.ID, Kind: base.Kind, State: base.State, Position: base.Position, Width: base.Width, Height: base.Height, Version: base.Version,
		AllowedActions: withoutRemove(base.AllowedActions), SnapshotID: b.SnapshotID, RunID: b.RunID,
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
		out.Panel = body.Panel
		var panel spec.Panel
		if json.Unmarshal(body.Panel, &panel) == nil {
			out.Title, out.Type, out.Datasources = panel.Title, string(panel.Type), panelDatasources(panel)
		}
		out.Variables = bakedValues(body.Variables)
		if body.From != nil && body.To != nil {
			out.From, out.To = body.From.UTC().Format(time.RFC3339), body.To.UTC().Format(time.RFC3339)
		}
	}
	return out
}

func withoutRemove(actions []string) []string {
	return slices.DeleteFunc(slices.Clone(actions), func(a string) bool { return a == string(observations.BlockRemove) })
}

func bakedValues(variables []spec.Variable) map[string][]string {
	out := map[string][]string{}
	for _, v := range variables {
		if v.Default != nil {
			out[v.Name] = v.Default.Values
		}
	}
	if len(out) == 0 {
		return nil
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

func checkListState(state observations.State) (Result, bool) {
	if state != "" && state != observations.StateOpen && state != observations.StateResolved {
		return invalid("state must be open or resolved"), false
	}
	return Result{}, true
}

type observationListIn struct {
	FolderID          string `json:"folder_id"`
	State             string `json:"state"`
	Title             string `json:"title"`
	Limit             *int   `json:"limit"`
	Cursor            string `json:"cursor"`
	IncludeRestricted bool   `json:"include_restricted"`
}

func (in observationListIn) checked() (observations.ListFilter, int, Result, bool) {
	if res, ok := checkLength("title", in.Title, maxNameBytes); !ok {
		return observations.ListFilter{}, 0, res, false
	}
	folderID, res, ok := parseFolderID(in.FolderID)
	if !ok {
		return observations.ListFilter{}, 0, res, false
	}
	filter := observations.ListFilter{FolderID: folderID, State: observations.State(in.State), Query: in.Title}
	if res, ok := checkListState(filter.State); !ok {
		return observations.ListFilter{}, 0, res, false
	}
	limit, res, ok := boundedInt(in.Limit, defaultListObservations, 1, maxListObservations, "limit")
	if !ok {
		return observations.ListFilter{}, 0, res, false
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return observations.ListFilter{}, 0, res, false
	}
	return filter, limit, Result{}, true
}

func (r *Registry) listObservations(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in observationListIn
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	filter, limit, res, ok := in.checked()
	if !ok {
		return res, nil
	}
	titles, res, ok, err := r.restrictedTitles(ctx, in.IncludeRestricted)
	if err != nil || !ok {
		return res, err
	}
	principal, _ := auth.FromContext(ctx)
	for {
		list, next, err := r.deps.Observations.List(ctx, principal, filter, limit, in.Cursor)
		if err != nil {
			return r.observationFailure(ctx, "Listed observations", err)
		}
		titles.restart()
		list = slices.DeleteFunc(list, func(o observations.Observation) bool { return !titles.keep(o.Folder.ID, o.ID) })
		out := observationListOut{Observations: make([]observations.ObservationJSON, len(list)), LeftOut: titles.leftOut, LeftOutWhy: titles.note("observation", "observations")}
		for i, o := range list {
			out.Observations[i] = observations.ObservationOf(o, r.deps.Observations.URL(o.ID))
		}
		if next != "" {
			out.NextCursor = &next
		}
		kept, cut := fit(out.Observations, listBudget-jsonSize(observationListOut{Observations: []observations.ObservationJSON{}, NextCursor: out.NextCursor, LeftOutWhy: out.LeftOutWhy}))
		if !cut || len(list) <= 1 {
			titles.done(ctx)
			return success("Listed "+plural(len(out.Observations), "observation", "observations"), out), nil
		}
		limit = max(len(kept), 1)
	}
}
