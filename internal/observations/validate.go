package observations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxTitleRunes    = 200
	MaxSummaryRunes  = 500
	MaxTextRunes     = 8000
	MaxBlocks        = 100
	MaxPanelBlocks   = 24
	MaxSnapshotBytes = 2 << 20
	MaxRange         = queryproxy.MaxRange

	BlockPanelID = "block"

	panelWrapperTitle = "Panel block"
	panelIssuePath    = "/panel"
	wrappedPanelPath  = "/panels/0"
	defaultPanelRange = time.Hour
	storedPrecision   = time.Microsecond
)

type TextBody struct {
	Markdown string `json:"markdown"`
}

type PanelBody struct {
	Panel json.RawMessage `json:"panel"`
	From  *time.Time      `json:"from,omitempty"`
	To    *time.Time      `json:"to,omitempty"`
}

func validateTitle(title string) (string, error) {
	trimmed := strings.TrimSpace(title)
	if n := utf8.RuneCountInString(trimmed); n < 1 || n > MaxTitleRunes {
		return "", invalid(CodeInvalidTitle, "title must be 1 to 200 characters")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", invalid(CodeInvalidTitle, "title must not contain the NUL character")
	}
	return trimmed, nil
}

func validateSummary(summary string) (string, error) {
	trimmed := strings.TrimSpace(summary)
	if n := utf8.RuneCountInString(trimmed); n < 1 || n > MaxSummaryRunes {
		return "", invalid(CodeInvalidSummary, "summary must be 1 to 500 characters")
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return "", invalid(CodeInvalidSummary, "summary must be one line")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", invalid(CodeInvalidSummary, "summary must not contain the NUL character")
	}
	return trimmed, nil
}

func validateRange(from, to *time.Time) error {
	switch {
	case from == nil && to == nil:
		return nil
	case from == nil || to == nil:
		return invalid(CodeInvalidRange, "from and to are given together or not at all")
	case !from.Before(*to):
		return invalid(CodeInvalidRange, "from must be before to")
	case to.Sub(*from) > MaxRange:
		return invalid(CodeInvalidRange, "the time range is longer than 400 days")
	}
	return nil
}

func decodeStrict(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func textBody(raw json.RawMessage) (TextBody, error) {
	var body struct {
		Markdown *string `json:"markdown"`
	}
	if err := decodeStrict(raw, &body); err != nil || body.Markdown == nil {
		return TextBody{}, invalid(CodeInvalidBlock, `a text block body is {"markdown": string}`)
	}
	markdown := *body.Markdown
	if strings.TrimSpace(markdown) == "" || utf8.RuneCountInString(markdown) > MaxTextRunes {
		return TextBody{}, invalid(CodeInvalidBlock, "markdown must be 1 to 8000 characters")
	}
	if strings.ContainsRune(markdown, 0) {
		return TextBody{}, invalid(CodeInvalidBlock, "markdown must not contain the NUL character")
	}
	return TextBody{Markdown: markdown}, nil
}

func panelBody(raw json.RawMessage) (PanelBody, error) {
	var body PanelBody
	if err := decodeStrict(raw, &body); err != nil || len(bytes.TrimSpace(body.Panel)) == 0 || body.Panel[0] != '{' {
		return PanelBody{}, invalid(CodeInvalidBlock, `a panel block body is {"panel": object, "from"?: time, "to"?: time}`)
	}
	if err := validateRange(body.From, body.To); err != nil {
		return PanelBody{}, err
	}
	if len(body.Panel) > spec.MaxSuggestedPanelBytes {
		return PanelBody{}, &InvalidInputError{
			Code:    CodeInvalidPanel,
			Message: fmt.Sprintf("the panel is larger than %d bytes", spec.MaxSuggestedPanelBytes),
			Issues:  []spec.Issue{{Path: panelIssuePath, Code: spec.CodeTooLarge, Message: "the panel is too large"}},
		}
	}
	if issues := panelIssues(body); len(issues) > 0 {
		return PanelBody{}, &InvalidInputError{Code: CodeInvalidPanel, Message: "the panel is invalid", Issues: issues}
	}
	body.Panel = compact(body.Panel)
	if body.From != nil {
		from, to := body.From.UTC(), body.To.UTC()
		body.From, body.To = &from, &to
	}
	return body, nil
}

func panelIssues(body PanelBody) []spec.Issue {
	from, to := validationRange(body)
	d, err := findings.EvidenceSpec(panelWrapperTitle, from, to, []json.RawMessage{body.Panel})
	var invalidSpec *spec.ValidationError
	switch {
	case errors.As(err, &invalidSpec):
		return repath(invalidSpec.Issues)
	case err != nil:
		return []spec.Issue{{Path: panelIssuePath, Code: spec.CodeSchema, Message: err.Error()}}
	}
	canonical, err := json.Marshal(d)
	if err != nil {
		return []spec.Issue{{Path: panelIssuePath, Code: spec.CodeSchema, Message: "the panel cannot be encoded"}}
	}
	if issues := spec.NULIssues(canonical); len(issues) > 0 {
		return repath(issues)
	}
	switch panel := d.Panels[0]; {
	case panel.Type == spec.Text:
		return []spec.Issue{{Path: panelIssuePath + "/type", Code: spec.CodeSchema, Message: "a text panel is not a panel block; use a text block"}}
	case !panel.Type.IsData():
		return []spec.Issue{{Path: panelIssuePath + "/type", Code: spec.CodeSchema, Message: fmt.Sprintf("a %s panel shows no data; a panel block needs a data panel", panel.Type)}}
	}
	return nil
}

func validationRange(body PanelBody) (time.Time, time.Time) {
	if body.From != nil && body.To != nil {
		return *body.From, *body.To
	}
	to := time.Now().UTC()
	return to.Add(-defaultPanelRange), to
}

func repath(issues []spec.Issue) []spec.Issue {
	out := make([]spec.Issue, len(issues))
	for i, issue := range issues {
		if rest, ok := strings.CutPrefix(issue.Path, wrappedPanelPath); ok {
			issue.Path = panelIssuePath + rest
		}
		out[i] = issue
	}
	return out
}

func compact(raw json.RawMessage) json.RawMessage {
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return raw
	}
	return out.Bytes()
}
