package grafana

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	actionDefaultTitle = "Run"
	actionTitleMax     = 100
	actionConfirmMax   = 500
	actionVariableMax  = 10
	actionVariableName = 100
	fieldActionMax     = 10
	actionPointer      = "/panels/0/display/elements/0/actions/0"

	actionURLReason         = "Actions run only against https addresses written out in full."
	actionAuthDroppedReason = "A credential was left out of this action; SlashObserve never stores credentials in a dashboard."
	actionReference         = "This action refers to a variable or value SlashObserve cannot fill in, so it was left out."
	actionBodyReason        = "An action body holds at most 10000 characters, so the action was left out."
	actionInvalidReason     = "This action cannot be kept as written, so it was left out."
	actionTypeReason        = "Only HTTP fetch actions are supported."
	actionMethodReason      = "This HTTP method is not supported yet."
	actionHeaderReason      = "A header or query parameter that cannot be kept was left out of this action."
	actionInputReason       = "This action input cannot be kept."
	actionInputLimitReason  = "An action holds at most 10 inputs."
	fieldActionLimitReason  = "A field holds at most 10 actions."
	oneClickTargetReason    = "Only one link or action can open on a single click."
)

var (
	actionMethods      = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	actionContentTypes = []string{"application/json", "text/plain", "application/x-www-form-urlencoded"}
	actionVariableKey  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	dataLinkTokens     = []string{"${__data.", "${__value", "${__series", "${__field"}
)

type actionParts struct {
	title       string
	method      string
	url         string
	contentType string
	body        string
	confirm     string
	color       string
	headers     []spec.ActionParam
	query       []spec.ActionParam
	variables   []spec.ActionVariable
}

var fetchActionKeys = []string{
	".type", ".title", ".confirmation", ".oneClick", ".variables",
	".fetch.method", ".fetch.url", ".fetch.contentType", ".fetch.body", ".fetch.data",
	".fetch.headers", ".fetch.headerParams", ".fetch.queryParams",
}

func (b *panelBuild) fetchAction(read, report string, raw map[string]any, drawsColor bool) (spec.Action, bool) {
	if kind := asString(raw["type"]); kind != "" && kind != "fetch" {
		b.consume(read)
		b.issues.add(report, actionTypeReason)
		return spec.Action{}, false
	}
	for _, key := range fetchActionKeys {
		b.consume(read + key)
	}
	fetch, _ := asObject(raw["fetch"])
	parts := actionParts{
		title:       asString(raw["title"]),
		method:      asString(fetch["method"]),
		url:         asString(fetch["url"]),
		contentType: asString(fetch["contentType"]),
		body:        firstText(fetch["body"], fetch["data"]),
		confirm:     asString(raw["confirmation"]),
		headers:     b.actionParams(read+".fetch.headers", firstPresent(fetch["headers"], fetch["headerParams"])),
		query:       b.actionParams(read+".fetch.queryParams", fetch["queryParams"]),
		variables:   b.actionVariables(report, raw["variables"]),
	}
	if drawsColor {
		parts.color = b.actionColor(read, report, raw)
	}
	return b.finishAction(report, parts)
}

func (b *panelBuild) actionVariables(report string, v any) []spec.ActionVariable {
	items, _ := asArray(v)
	var out []spec.ActionVariable
	for i, item := range items {
		at := fmt.Sprintf("%s.variables[%d]", report, i)
		variable, ok := actionVariableOf(item)
		switch {
		case !ok || slices.ContainsFunc(out, func(o spec.ActionVariable) bool { return o.Key == variable.Key }):
			b.issues.add(at, actionInputReason)
		case len(out) >= actionVariableMax:
			b.issues.add(at, actionInputLimitReason)
		default:
			out = append(out, variable)
		}
	}
	return out
}

func actionVariableOf(item any) (spec.ActionVariable, bool) {
	m, ok := asObject(item)
	if !ok {
		return spec.ActionVariable{}, false
	}
	key := asString(m["key"])
	if !actionVariableKey.MatchString(key) {
		return spec.ActionVariable{}, false
	}
	if kind := asString(m["type"]); kind != "" && kind != "string" {
		return spec.ActionVariable{}, false
	}
	name := asString(m["name"])
	if name == "" {
		name = key
	}
	if utf8.RuneCountInString(name) > actionVariableName {
		return spec.ActionVariable{}, false
	}
	return spec.ActionVariable{Key: key, Name: name}, true
}

func (b *panelBuild) actionColor(read, report string, raw map[string]any) string {
	text := strings.TrimSpace(asString(lookupValue(raw, "style.backgroundColor")))
	if text == "" {
		return ""
	}
	b.consume(read + ".style.backgroundColor")
	color, ok := seriesColor(text)
	if !ok {
		b.issues.add(report+".style.backgroundColor", fmt.Sprintf("The color %q is not supported yet.", text))
	}
	return color
}

func propActions(b *panelBuild, p *spec.FieldProps, v any, s propSite) {
	items, _ := asArray(v)
	var actions []spec.Action
	for i, item := range items {
		read := fmt.Sprintf("%s[%d]", s.value, i)
		report := s.at(fmt.Sprintf("[%d]", i))
		raw, _ := asObject(item)
		action, ok := b.fetchAction(read, report, raw, true)
		switch {
		case !ok:
		case len(actions) >= fieldActionMax:
			b.issues.add(report, fieldActionLimitReason)
		default:
			action.OneClick = truthy(raw["oneClick"])
			actions = append(actions, action)
		}
	}
	if len(actions) > 0 {
		p.Actions = actions
	}
	b.settleOneClick(p, s.at(""))
}

func isDataLink(l spec.Link) bool {
	return l.OneClick || slices.ContainsFunc(dataLinkTokens, func(token string) bool {
		return strings.Contains(l.URL, token) || strings.Contains(l.Title, token)
	})
}

func (b *panelBuild) settleOneClick(p *spec.FieldProps, at string) {
	if len(p.Actions) == 0 {
		return
	}
	clicks := slices.ContainsFunc(p.Actions, func(a spec.Action) bool { return a.OneClick }) ||
		slices.ContainsFunc(p.Links, func(l spec.Link) bool { return l.OneClick })
	total := len(p.Actions) + len(slices.DeleteFunc(slices.Clone(p.Links), func(l spec.Link) bool { return !isDataLink(l) }))
	if !clicks || total == 1 {
		return
	}
	for i := range p.Actions {
		p.Actions[i].OneClick = false
	}
	for i := range p.Links {
		p.Links[i].OneClick = false
	}
	b.issues.add(at, oneClickTargetReason)
}

func firstText(values ...any) string {
	for _, v := range values {
		if text := asString(v); text != "" {
			return text
		}
	}
	return ""
}

func firstPresent(values ...any) any {
	for _, v := range values {
		if present(v) {
			return v
		}
	}
	return nil
}

func (b *panelBuild) actionParams(at string, v any) []spec.ActionParam {
	items, _ := asArray(v)
	var params []spec.ActionParam
	for _, item := range items {
		switch pair := item.(type) {
		case []any:
			if len(pair) == 2 {
				params = append(params, spec.ActionParam{Name: asString(pair[0]), Value: asString(pair[1])})
			}
		case map[string]any:
			params = append(params, spec.ActionParam{Name: asString(pair["name"]), Value: asString(pair["value"])})
		}
	}
	return params
}

func (b *panelBuild) finishAction(at string, parts actionParts) (spec.Action, bool) {
	action, ok := b.actionOf(at, parts)
	if !ok {
		return spec.Action{}, false
	}
	for range 2 {
		issues := b.actionIssues(action)
		if len(issues) == 0 {
			return action, true
		}
		var dropped bool
		action, dropped = b.withoutBadParams(at, action, issues)
		if !dropped {
			b.issues.add(at, actionReason(issues))
			return spec.Action{}, false
		}
	}
	b.issues.add(at, actionInvalidReason)
	return spec.Action{}, false
}

func (b *panelBuild) actionOf(at string, parts actionParts) (spec.Action, bool) {
	action := spec.Action{Title: strings.TrimSpace(parts.title), URL: strings.TrimSpace(parts.url), Body: parts.body}
	if action.Title == "" {
		action.Title = actionDefaultTitle
	}
	action.Title, _ = cutRunes(action.Title, actionTitleMax)
	if confirm := strings.TrimSpace(parts.confirm); confirm != "" {
		action.Confirm, _ = cutRunes(confirm, actionConfirmMax)
	}
	if method := strings.ToUpper(strings.TrimSpace(parts.method)); method != "" && method != "POST" {
		if !slices.Contains(actionMethods, method) {
			b.issues.add(at, actionMethodReason)
			return spec.Action{}, false
		}
		action.Method = method
	}
	action.Headers = parts.headers
	action.Query = parts.query
	action.Variables = parts.variables
	action.Color = parts.color
	action.ContentType = b.actionContentType(at, parts.contentType, &action)
	return action, true
}

func (b *panelBuild) actionContentType(at, declared string, action *spec.Action) string {
	contentType := strings.ToLower(strings.TrimSpace(declared))
	action.Headers = slices.DeleteFunc(slices.Clone(action.Headers), func(p spec.ActionParam) bool {
		if !strings.EqualFold(p.Name, "content-type") {
			return false
		}
		contentType = strings.ToLower(strings.TrimSpace(p.Value))
		return true
	})
	switch {
	case contentType == "" || contentType == "application/json":
		return ""
	case slices.Contains(actionContentTypes, contentType):
		return contentType
	}
	b.issues.add(at, "This content type is not supported yet, so JSON is used.")
	return ""
}

func (b *panelBuild) actionIssues(action spec.Action) []spec.Issue {
	variables := make([]map[string]any, 0, len(b.c.vars))
	for _, v := range b.c.vars {
		variables = append(variables, map[string]any{"name": v.v.Name, "type": "textbox"})
	}
	data, err := json.Marshal(map[string]any{
		"specVersion": spec.Version,
		"title":       "Action",
		"time":        map[string]any{"from": "now-15m", "to": "now"},
		"variables":   variables,
		"panels": []any{map[string]any{
			"id": "p1", "type": "canvas", "title": "Canvas",
			"gridPos":    map[string]any{"x": 0, "y": 0, "w": 12, "h": 8},
			"datasource": "ds", "queries": []any{map[string]any{"query": "up"}},
			"display": map[string]any{"elements": []any{map[string]any{
				"id": "b", "kind": "button", "left": 1, "top": 1, "width": 10, "height": 10,
				"actions": []any{action},
			}}},
		}},
	})
	if err != nil {
		return []spec.Issue{{Path: actionPointer}}
	}
	_, err = spec.Parse(data)
	var invalid *spec.ValidationError
	if !errors.As(err, &invalid) {
		if err != nil {
			return []spec.Issue{{Path: actionPointer}}
		}
		return nil
	}
	return slices.DeleteFunc(slices.Clone(invalid.Issues), func(issue spec.Issue) bool {
		return !strings.HasPrefix(issue.Path, actionPointer)
	})
}

func (b *panelBuild) withoutBadParams(at string, action spec.Action, issues []spec.Issue) (spec.Action, bool) {
	badHeaders, badQuery := map[int]bool{}, map[int]bool{}
	credential := false
	for _, issue := range issues {
		parts := strings.Split(strings.TrimPrefix(issue.Path, actionPointer), "/")
		if len(parts) < 3 {
			continue
		}
		var index int
		if _, err := fmt.Sscanf(parts[2], "%d", &index); err != nil {
			continue
		}
		switch parts[1] {
		case "headers":
			badHeaders[index] = true
		case "query":
			badQuery[index] = true
		default:
			continue
		}
		credential = credential || issue.Code == spec.CodeActionHeaderNotAllowed
	}
	if len(badHeaders)+len(badQuery) == 0 {
		return action, false
	}
	action.Headers = keepUnmarked(action.Headers, badHeaders)
	action.Query = keepUnmarked(action.Query, badQuery)
	if credential {
		b.issues.add(at, actionAuthDroppedReason)
	} else {
		b.issues.add(at, actionHeaderReason)
	}
	return action, true
}

func keepUnmarked(params []spec.ActionParam, marked map[int]bool) []spec.ActionParam {
	var kept []spec.ActionParam
	for i, p := range params {
		if !marked[i] {
			kept = append(kept, p)
		}
	}
	return kept
}

func actionReason(issues []spec.Issue) string {
	for _, issue := range issues {
		field := strings.TrimPrefix(issue.Path, actionPointer)
		switch {
		case issue.Code == spec.CodeUnknownVariable || issue.Code == spec.CodeInvalidReference:
			return actionReference
		case field == "/url":
			return actionURLReason
		case field == "/body":
			return actionBodyReason
		}
	}
	return actionInvalidReason
}
