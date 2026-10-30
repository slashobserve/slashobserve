package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ActionUser struct {
	ID    string
	Email string
	Name  string
}

type ActionValue struct {
	Raw     string
	Text    string
	Numeric *float64
}

type ActionEnv struct {
	Variables map[string][]string
	From      time.Time
	To        time.Time
	User      ActionUser
	Value     ActionValue
	Fields    map[string]string
}

type ExpandedAction struct {
	Method string
	URL    string
	Host   string
	Header http.Header
	Body   []byte
}

type valueEscaper func(string) string

var valueFormats = map[string]func(values []string) string{
	"":            joinWith(","),
	"csv":         joinWith(","),
	"raw":         joinWith(","),
	"pipe":        joinWith("|"),
	"regex":       regexAlternatives,
	"singlequote": quoteEach(`'`, strings.NewReplacer(`\`, `\\`, `'`, `\'`)),
	"doublequote": quoteEach(`"`, strings.NewReplacer(`\`, `\\`, `"`, `\"`)),
	"sqlstring":   quoteEach(`'`, strings.NewReplacer(`'`, `''`)),
}

func joinWith(sep string) func([]string) string {
	return func(values []string) string { return strings.Join(values, sep) }
}

func quoteEach(quote string, escaper *strings.Replacer) func([]string) string {
	return func(values []string) string {
		quoted := make([]string, len(values))
		for i, v := range values {
			quoted[i] = quote + escaper.Replace(v) + quote
		}
		return strings.Join(quoted, ",")
	}
}

func regexAlternatives(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = regexp.QuoteMeta(v)
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return "(" + strings.Join(quoted, "|") + ")"
}

func ExpandAction(a Action, env ActionEnv) (ExpandedAction, error) {
	defaults := Defaults().Action
	method := orDefault(a.Method, defaults.Method)
	contentType := orDefault(a.ContentType, defaults.ContentType)

	target, host, err := expandActionURL(a, env)
	if err != nil {
		return ExpandedAction{}, err
	}
	header, err := expandActionHeaders(a.Headers, env)
	if err != nil {
		return ExpandedAction{}, err
	}
	body, err := expandActionBody(a.Body, contentType, env)
	if err != nil {
		return ExpandedAction{}, err
	}
	if body != nil {
		header.Set("Content-Type", contentType)
	}
	return ExpandedAction{Method: method, URL: target, Host: host, Header: header, Body: body}, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func expandActionURL(a Action, env ActionEnv) (string, string, error) {
	template, err := url.Parse(a.URL)
	if err != nil {
		return "", "", errors.New("action url is not a valid address")
	}
	queryStart := strings.IndexByte(a.URL, '?')
	expanded, err := expandActionTemplate(a.URL, env, func(offset int) valueEscaper {
		if queryStart >= 0 && offset > queryStart {
			return url.QueryEscape
		}
		return url.PathEscape
	})
	if err != nil {
		return "", "", err
	}
	u, err := url.Parse(expanded)
	if err != nil {
		return "", "", errors.New("action url is not a valid address after expansion")
	}
	if u.Scheme != template.Scheme || u.User != nil || !strings.EqualFold(u.Host, template.Host) {
		return "", "", errors.New("action url changed its scheme or host after expansion")
	}
	var extra []string
	for _, p := range a.Query {
		value, err := expandActionTemplate(p.Value, env, constantEscaper(identity))
		if err != nil {
			return "", "", err
		}
		extra = append(extra, url.QueryEscape(p.Name)+"="+url.QueryEscape(value))
	}
	if len(extra) > 0 {
		if u.RawQuery != "" {
			extra = append([]string{u.RawQuery}, extra...)
		}
		u.RawQuery = strings.Join(extra, "&")
	}
	return u.String(), strings.ToLower(u.Hostname()), nil
}

func expandActionHeaders(params []ActionParam, env ActionEnv) (http.Header, error) {
	header := http.Header{}
	for _, p := range params {
		value, err := expandActionTemplate(p.Value, env, constantEscaper(identity))
		if err != nil {
			return nil, err
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("header %q holds a line break or NUL after expansion", p.Name)
		}
		header.Add(p.Name, value)
	}
	return header, nil
}

func expandActionBody(template, contentType string, env ActionEnv) ([]byte, error) {
	if template == "" {
		return nil, nil
	}
	var escape valueEscaper
	switch contentType {
	case "application/json":
		escape = jsonStringContent
	case "application/x-www-form-urlencoded":
		escape = url.QueryEscape
	case "text/plain":
		escape = identity
	default:
		return nil, fmt.Errorf("unsupported action content type %q", contentType)
	}
	body, err := expandActionTemplate(template, env, constantEscaper(escape))
	if err != nil {
		return nil, err
	}
	if contentType == "application/json" && !json.Valid([]byte(body)) {
		return nil, errors.New("action body is not valid JSON after expansion")
	}
	return []byte(body), nil
}

func identity(s string) string { return s }

func constantEscaper(e valueEscaper) func(int) valueEscaper {
	return func(int) valueEscaper { return e }
}

func jsonStringContent(s string) string {
	quoted, _ := json.Marshal(s)
	return string(quoted[1 : len(quoted)-1])
}

func expandActionTemplate(template string, env ActionEnv, escaperAt func(offset int) valueEscaper) (string, error) {
	refs, err := ActionReferences(template)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	last := 0
	for _, ref := range refs {
		value, err := env.resolve(ref)
		if err != nil {
			return "", err
		}
		out.WriteString(template[last:ref.Start])
		out.WriteString(escaperAt(ref.Start)(value))
		last = ref.End
	}
	out.WriteString(template[last:])
	return out.String(), nil
}

func (e ActionEnv) resolve(ref Reference) (string, error) {
	switch ref.Name {
	case "__from":
		return FormatTimeMacro(e.From, ref.Format)
	case "__to":
		return FormatTimeMacro(e.To, ref.Format)
	case "__user.login", "__user.email":
		return formatReference(ref, []string{e.User.Email})
	case "__user.name":
		return formatReference(ref, []string{e.User.Name})
	case "__user.id":
		return formatReference(ref, []string{e.User.ID})
	case "__value.raw":
		return formatReference(ref, []string{e.Value.Raw})
	case "__value.text":
		return formatReference(ref, []string{e.Value.Text})
	case "__value.numeric":
		if e.Value.Numeric == nil {
			return "", fmt.Errorf("%s has no value", ref.Name)
		}
		return formatReference(ref, []string{strconv.FormatFloat(*e.Value.Numeric, 'f', -1, 64)})
	}
	if field, ok := strings.CutPrefix(ref.Name, dataFieldsPrefix); ok {
		value, found := e.Fields[field]
		if !found {
			return "", fmt.Errorf("%s has no value", ref.Name)
		}
		return formatReference(ref, []string{value})
	}
	values, found := e.Variables[ref.Name]
	if !found || len(values) == 0 {
		return "", fmt.Errorf("variable %q has no value", ref.Name)
	}
	return formatReference(ref, values)
}

func formatReference(ref Reference, values []string) (string, error) {
	format, ok := valueFormats[ref.Format]
	if !ok {
		return "", fmt.Errorf("unknown format %q in reference to %q", ref.Format, ref.Name)
	}
	return format(values), nil
}
