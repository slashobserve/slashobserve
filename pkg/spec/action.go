package spec

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const dataFieldsPrefix = "__data.fields."

var actionBuiltins = []string{
	"__from", "__to",
	"__user.login", "__user.email", "__user.name", "__user.id",
	"__value.raw", "__value.text", "__value.numeric",
}

var forbiddenActionParamNames = []string{
	"authorization", "proxy-authorization", "cookie", "set-cookie", "host", "content-type", "content-length",
	"transfer-encoding", "connection", "keep-alive", "te", "trailer", "upgrade", "expect",
}

var credentialNameParts = []string{"token", "secret", "password", "apikey", "api-key", "api_key", "auth", "session", "signature"}

const actionParamMessage = "credentials and transport headers are never stored in a dashboard; the server adds what it needs"

func ActionReferences(template string) ([]Reference, error) {
	var refs []Reference
	for i := 0; i < len(template); {
		if template[i] != '$' {
			i++
			continue
		}
		ref, next, err := scanActionReference(template, i)
		if err != nil {
			return nil, err
		}
		if ref != nil {
			refs = append(refs, *ref)
		}
		i = next
	}
	return refs, nil
}

func scanActionReference(template string, start int) (*Reference, int, error) {
	rest := template[start+1:]
	if strings.HasPrefix(rest, "{") {
		closing := strings.IndexByte(rest, '}')
		if closing < 0 {
			return nil, 0, scanErrorf(start, "unterminated \"${\" at offset ")
		}
		end := start + 1 + closing + 1
		name, format, hasFormat := strings.Cut(rest[1:closing], ":")
		if name == "" || (hasFormat && format == "") {
			return nil, 0, scanErrorf(start, "invalid reference %q at offset ", template[start:end])
		}
		return &Reference{Kind: referenceKind(name), Name: name, Format: format, Start: start, End: end}, end, nil
	}
	n := identLen(rest)
	if n == 0 {
		return nil, start + 1, nil
	}
	name := rest[:n]
	return &Reference{Kind: referenceKind(name), Name: name, Start: start, End: start + 1 + n}, start + 1 + n, nil
}

func forbiddenActionParam(name string) bool {
	lower := strings.ToLower(name)
	if slices.Contains(forbiddenActionParamNames, lower) {
		return true
	}
	return slices.ContainsFunc(credentialNameParts, func(part string) bool { return strings.Contains(lower, part) })
}

func actionBuiltin(name string) bool {
	if slices.Contains(actionBuiltins, name) {
		return true
	}
	field, ok := strings.CutPrefix(name, dataFieldsPrefix)
	return ok && field != ""
}

func checkActions(d *Dashboard, entries []PanelEntry) []Issue {
	declared := map[string]bool{}
	for _, v := range d.Variables {
		declared[v.Name] = true
	}
	var issues []Issue
	for _, e := range entries {
		if e.Panel.Type != Canvas || e.Panel.Display == nil {
			continue
		}
		for i, el := range e.Panel.Display.Elements {
			for j, a := range el.Actions {
				base := e.Pointer + pointer("display", "elements", strconv.Itoa(i), "actions", strconv.Itoa(j))
				issues = append(issues, checkAction(declared, a, base)...)
			}
		}
	}
	return issues
}

func checkAction(declared map[string]bool, a Action, base string) []Issue {
	var issues []Issue
	issues = append(issues, checkActionTemplate(declared, a.URL, base+"/url")...)
	issues = append(issues, checkActionTemplate(declared, a.Body, base+"/body")...)
	for _, group := range []struct {
		name   string
		params []ActionParam
	}{{"headers", a.Headers}, {"query", a.Query}} {
		for k, p := range group.params {
			at := base + pointer(group.name, strconv.Itoa(k))
			if forbiddenActionParam(p.Name) {
				issues = append(issues, Issue{
					Path:    at + "/name",
					Code:    CodeActionHeaderNotAllowed,
					Message: actionParamMessage,
				})
			}
			issues = append(issues, checkActionTemplate(declared, p.Value, at+"/value")...)
		}
	}
	return issues
}

func checkActionTemplate(declared map[string]bool, template, path string) []Issue {
	refs, err := ActionReferences(template)
	if err != nil {
		return []Issue{{Path: path, Code: CodeInvalidReference, Message: err.Error()}}
	}
	var issues []Issue
	for _, ref := range refs {
		switch {
		case !declared[ref.Name] && !actionBuiltin(ref.Name):
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeUnknownVariable,
				Message: fmt.Sprintf("action references undeclared variable %q; declare it in variables or remove the reference", ref.Name),
			})
		case ref.Format != "" && !slices.Contains(VariableFormats(), ref.Format):
			issues = append(issues, Issue{
				Path:    path,
				Code:    CodeUnknownVariable,
				Message: fmt.Sprintf("unknown format %q in reference to %q; use one of %s", ref.Format, ref.Name, strings.Join(VariableFormats(), ", ")),
			})
		}
	}
	return issues
}
