package grafana

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	canvasLinkMax            = 10
	canvasActionMax          = 10
	canvasDefaultActionTitle = "Run"
	canvasActionTitleMax     = 100
	canvasActionConfirmMax   = 500
	canvasActionPointer      = "/panels/0/display/elements/0/actions/0"

	canvasLinkLimitReason        = "A canvas element holds at most 10 links."
	canvasActionLimitReason      = "A canvas element holds at most 10 actions."
	canvasActionURLReason        = "Actions run only against https addresses written out in full."
	canvasActionCredentialReason = "A credential was left out of this action; SlashObserve never stores credentials in a dashboard."
	canvasActionReference        = "This action refers to a variable or value SlashObserve cannot fill in, so it was left out."
	canvasActionBodyReason       = "An action body holds at most 10000 characters, so the action was left out."
	canvasActionInvalidReason    = "This action cannot be kept as written, so it was left out."
	canvasActionTypeReason       = "Only HTTP fetch actions are supported."
	canvasActionMethodReason     = "This HTTP method is not supported yet."
	canvasActionHeaderReason     = "A header or query parameter that cannot be kept was left out of this action."
	canvasOneClickReason         = "Only one link or action can open on a single click."
)

var (
	canvasActionMethods      = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	canvasActionContentTypes = []string{"application/json", "text/plain", "application/x-www-form-urlencoded"}
)

type actionParts struct {
	title       string
	method      string
	url         string
	contentType string
	body        string
	confirm     string
	headers     []spec.ActionParam
	query       []spec.ActionParam
}

func (c *canvasBuild) elementLinks(out *spec.CanvasElement, el map[string]any, path string) {
	items, _ := asArray(el["links"])
	oneClick := false
	for i, item := range items {
		at := fmt.Sprintf("%s.links[%d]", path, i)
		link, reason := linkOf(item)
		switch {
		case reason != "":
			c.b.partial(at, reason)
		case len(out.Links) >= canvasLinkMax:
			c.b.partial(at, canvasLinkLimitReason)
		default:
			oneClick = oneClick || link.OneClick
			link.OneClick = false
			out.Links = append(out.Links, link)
			c.b.consume(at)
		}
	}
	if oneClick {
		out.OneClick = "link"
	}
}

func (c *canvasBuild) elementActions(out *spec.CanvasElement, el map[string]any, path string) {
	items, _ := asArray(el["actions"])
	oneClick := false
	for i, item := range items {
		at := fmt.Sprintf("%s.actions[%d]", path, i)
		raw, _ := asObject(item)
		action, ok := c.fetchAction(at, raw)
		switch {
		case !ok:
		case len(out.Actions) >= canvasActionMax:
			c.b.partial(at, canvasActionLimitReason)
		default:
			out.Actions = append(out.Actions, action)
			oneClick = oneClick || truthy(raw["oneClick"])
		}
	}
	if oneClick && out.OneClick == "" {
		out.OneClick = "action"
	} else if oneClick {
		c.b.issues.add(path+".actions", canvasOneClickReason)
	}
}

func (c *canvasBuild) fetchAction(at string, raw map[string]any) (spec.Action, bool) {
	c.b.consume(at)
	if kind := asString(raw["type"]); kind != "" && kind != "fetch" {
		c.b.issues.add(at, canvasActionTypeReason)
		return spec.Action{}, false
	}
	fetch, _ := asObject(raw["fetch"])
	parts := actionParts{
		title:       asString(raw["title"]),
		method:      asString(fetch["method"]),
		url:         asString(fetch["url"]),
		contentType: asString(fetch["contentType"]),
		body:        firstText(fetch["body"], fetch["data"]),
		confirm:     asString(raw["confirmation"]),
		headers:     c.actionParams(at+".fetch.headers", firstPresent(fetch["headers"], fetch["headerParams"])),
		query:       c.actionParams(at+".fetch.queryParams", fetch["queryParams"]),
	}
	return c.finishAction(at, parts)
}

func (c *canvasBuild) buttonAction(out *spec.CanvasElement, el map[string]any, path string) {
	api, ok := asObject(lookupValue(el, "config.api"))
	if !ok || asString(api["endpoint"]) == "" {
		return
	}
	at := path + ".config.api"
	c.b.consume(at)
	parts := actionParts{
		title:       out.Text,
		method:      asString(api["method"]),
		url:         asString(api["endpoint"]),
		contentType: asString(api["contentType"]),
		body:        asString(api["data"]),
		headers:     c.actionParams(at+".headerParams", api["headerParams"]),
		query:       c.actionParams(at+".queryParams", api["queryParams"]),
	}
	if action, valid := c.finishAction(at, parts); valid && len(out.Actions) < canvasActionMax {
		out.Actions = append(out.Actions, action)
	}
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

func (c *canvasBuild) actionParams(at string, v any) []spec.ActionParam {
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

func (c *canvasBuild) finishAction(at string, parts actionParts) (spec.Action, bool) {
	action, ok := c.actionOf(at, parts)
	if !ok {
		return spec.Action{}, false
	}
	for range 2 {
		issues := c.actionIssues(action)
		if len(issues) == 0 {
			return action, true
		}
		var dropped bool
		action, dropped = c.withoutBadParams(at, action, issues)
		if !dropped {
			c.b.issues.add(at, actionReason(issues))
			return spec.Action{}, false
		}
	}
	c.b.issues.add(at, canvasActionInvalidReason)
	return spec.Action{}, false
}

func (c *canvasBuild) actionOf(at string, parts actionParts) (spec.Action, bool) {
	action := spec.Action{Title: strings.TrimSpace(parts.title), URL: strings.TrimSpace(parts.url), Body: parts.body}
	if action.Title == "" {
		action.Title = canvasDefaultActionTitle
	}
	action.Title, _ = cutRunes(action.Title, canvasActionTitleMax)
	if confirm := strings.TrimSpace(parts.confirm); confirm != "" {
		action.Confirm, _ = cutRunes(confirm, canvasActionConfirmMax)
	}
	if method := strings.ToUpper(strings.TrimSpace(parts.method)); method != "" && method != "POST" {
		if !slices.Contains(canvasActionMethods, method) {
			c.b.issues.add(at, canvasActionMethodReason)
			return spec.Action{}, false
		}
		action.Method = method
	}
	action.Headers = parts.headers
	action.Query = parts.query
	action.ContentType = c.actionContentType(at, parts.contentType, &action)
	return action, true
}

func (c *canvasBuild) actionContentType(at, declared string, action *spec.Action) string {
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
	case slices.Contains(canvasActionContentTypes, contentType):
		return contentType
	}
	c.b.issues.add(at, "This content type is not supported yet, so JSON is used.")
	return ""
}

func (c *canvasBuild) actionIssues(action spec.Action) []spec.Issue {
	variables := make([]map[string]any, 0, len(c.b.c.vars))
	for _, v := range c.b.c.vars {
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
		return []spec.Issue{{Path: canvasActionPointer}}
	}
	_, err = spec.Parse(data)
	var invalid *spec.ValidationError
	if !errors.As(err, &invalid) {
		if err != nil {
			return []spec.Issue{{Path: canvasActionPointer}}
		}
		return nil
	}
	return slices.DeleteFunc(slices.Clone(invalid.Issues), func(issue spec.Issue) bool {
		return !strings.HasPrefix(issue.Path, canvasActionPointer)
	})
}

func (c *canvasBuild) withoutBadParams(at string, action spec.Action, issues []spec.Issue) (spec.Action, bool) {
	badHeaders, badQuery := map[int]bool{}, map[int]bool{}
	credential := false
	for _, issue := range issues {
		parts := strings.Split(strings.TrimPrefix(issue.Path, canvasActionPointer), "/")
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
		c.b.issues.add(at, canvasActionCredentialReason)
	} else {
		c.b.issues.add(at, canvasActionHeaderReason)
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
		field := strings.TrimPrefix(issue.Path, canvasActionPointer)
		switch {
		case issue.Code == spec.CodeUnknownVariable || issue.Code == spec.CodeInvalidReference:
			return canvasActionReference
		case field == "/url":
			return canvasActionURLReason
		case field == "/body":
			return canvasActionBodyReason
		}
	}
	return canvasActionInvalidReason
}
