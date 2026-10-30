package spec_test

import (
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestExpandActionBuildsRequestsFromTemplates(t *testing.T) {
	twelve, fractional := 12.0, 1.5
	env := spec.ActionEnv{
		Variables: map[string][]string{
			"name":  {"a/b c"},
			"env":   {"say \"hi\"\n"},
			"amp":   {"a&b"},
			"multi": {"x y", "z"},
			"quote": {"it's"},
			"plain": {"prod", "stage"},
		},
		From:   time.UnixMilli(1_700_000_000_000),
		To:     time.UnixMilli(1_700_000_060_000),
		User:   spec.ActionUser{ID: "u-1", Email: "ada@example.com", Name: "Ada Lovelace"},
		Value:  spec.ActionValue{Raw: "12", Text: "12 req/s", Numeric: &twelve},
		Fields: map[string]string{"status": "down"},
	}
	cases := []struct {
		name       string
		action     spec.Action
		env        spec.ActionEnv
		wantMethod string
		wantURL    string
		wantHost   string
		wantBody   string
		wantType   string
		wantHeader map[string]string
	}{
		{
			name:       "defaults a missing method and content type",
			action:     spec.Action{URL: "https://API.Example.com/run", Body: `{"a":1}`},
			env:        env,
			wantMethod: "POST", wantURL: "https://API.Example.com/run", wantHost: "api.example.com",
			wantBody: `{"a":1}`, wantType: "application/json",
		},
		{
			name:       "escapes a path value with a slash and a space",
			action:     spec.Action{Method: "GET", URL: "https://api.example.com/svc/${name}/restart"},
			env:        env,
			wantMethod: "GET", wantURL: "https://api.example.com/svc/a%2Fb%20c/restart", wantHost: "api.example.com",
		},
		{
			name:       "escapes a value inside the URL's own query",
			action:     spec.Action{Method: "GET", URL: "https://api.example.com/run?env=${amp}"},
			env:        env,
			wantMethod: "GET", wantURL: "https://api.example.com/run?env=a%26b", wantHost: "api.example.com",
		},
		{
			name: "appends query parameters after the URL's own query",
			action: spec.Action{Method: "GET", URL: "https://api.example.com/run?a=1", Query: []spec.ActionParam{
				{Name: "q", Value: "${multi}"}, {Name: "who", Value: "${__user.login}"},
			}},
			env:        env,
			wantMethod: "GET", wantURL: "https://api.example.com/run?a=1&q=x+y%2Cz&who=ada%40example.com", wantHost: "api.example.com",
		},
		{
			name:       "keeps a JSON body valid when a value holds a quote and a newline",
			action:     spec.Action{URL: "https://api.example.com/run", Body: `{"env":"${env}"}`},
			env:        env,
			wantMethod: "POST", wantURL: "https://api.example.com/run", wantHost: "api.example.com",
			wantBody: `{"env":"say \"hi\"\n"}`, wantType: "application/json",
		},
		{
			name:       "accepts a bare JSON number reference",
			action:     spec.Action{URL: "https://api.example.com/run", Body: `{"n":${__value.raw}}`},
			env:        env,
			wantMethod: "POST", wantURL: "https://api.example.com/run", wantHost: "api.example.com",
			wantBody: `{"n":12}`, wantType: "application/json",
		},
		{
			name:       "form-encodes values in a form body",
			action:     spec.Action{URL: "https://api.example.com/run", ContentType: "application/x-www-form-urlencoded", Body: "a=${amp}&b=${__value.text}"},
			env:        env,
			wantMethod: "POST", wantURL: "https://api.example.com/run", wantHost: "api.example.com",
			wantBody: "a=a%26b&b=12+req%2Fs", wantType: "application/x-www-form-urlencoded",
		},
		{
			name:       "leaves a plain text body as written",
			action:     spec.Action{URL: "https://api.example.com/run", ContentType: "text/plain", Body: "env=${amp} status=${__data.fields.status}"},
			env:        env,
			wantMethod: "POST", wantURL: "https://api.example.com/run", wantHost: "api.example.com",
			wantBody: "env=a&b status=down", wantType: "text/plain",
		},
		{
			name: "applies formats to multi-value variables",
			action: spec.Action{URL: "https://api.example.com/run", ContentType: "text/plain",
				Body: "${plain} ${plain:csv} ${plain:pipe} ${plain:regex} ${quote:sqlstring} ${quote:singlequote} ${quote:doublequote}"},
			env:        env,
			wantMethod: "POST", wantURL: "https://api.example.com/run", wantHost: "api.example.com",
			wantBody: `prod,stage prod,stage prod|stage (prod|stage) 'it''s' 'it\'s' "it's"`, wantType: "text/plain",
		},
		{
			name: "expands user, value and time builtins in headers",
			action: spec.Action{Method: "PUT", URL: "https://api.example.com/run", Headers: []spec.ActionParam{
				{Name: "X-Email", Value: "${__user.email}"}, {Name: "X-Login", Value: "${__user.login}"},
				{Name: "X-Name", Value: "${__user.name}"}, {Name: "X-Id", Value: "${__user.id}"},
				{Name: "X-Num", Value: "${__value.numeric}"}, {Name: "X-From", Value: "${__from}"},
				{Name: "X-To", Value: "${__to:date:seconds}"},
			}},
			env:        env,
			wantMethod: "PUT", wantURL: "https://api.example.com/run", wantHost: "api.example.com",
			wantHeader: map[string]string{
				"X-Email": "ada@example.com", "X-Login": "ada@example.com", "X-Name": "Ada Lovelace", "X-Id": "u-1",
				"X-Num": "12", "X-From": "1700000000000", "X-To": "1700000060",
			},
		},
		{
			name:       "writes a fractional numeric value without an exponent",
			action:     spec.Action{Method: "PUT", URL: "https://api.example.com/run", Headers: []spec.ActionParam{{Name: "X-Num", Value: "${__value.numeric}"}}},
			env:        spec.ActionEnv{Value: spec.ActionValue{Numeric: &fractional}},
			wantMethod: "PUT", wantURL: "https://api.example.com/run", wantHost: "api.example.com",
			wantHeader: map[string]string{"X-Num": "1.5"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := spec.ExpandAction(tc.action, tc.env)
			if err != nil {
				t.Fatal(err)
			}
			if got.Method != tc.wantMethod || got.URL != tc.wantURL || got.Host != tc.wantHost || string(got.Body) != tc.wantBody {
				t.Errorf("got %s %s host %s body %q, want %s %s host %s body %q",
					got.Method, got.URL, got.Host, got.Body, tc.wantMethod, tc.wantURL, tc.wantHost, tc.wantBody)
			}
			if got.Header.Get("Content-Type") != tc.wantType {
				t.Errorf("Content-Type = %q, want %q", got.Header.Get("Content-Type"), tc.wantType)
			}
			for name, want := range tc.wantHeader {
				if got.Header.Get(name) != want {
					t.Errorf("header %s = %q, want %q", name, got.Header.Get(name), want)
				}
			}
		})
	}
}

func TestExpandActionRefusesWhatItCannotBuildSafely(t *testing.T) {
	env := spec.ActionEnv{
		Variables: map[string][]string{"v": {"bad\r\nX-Evil: 1"}, "word": {"abc"}, "host": {"evil.com"}, "empty": {}},
	}
	cases := []struct {
		name   string
		action spec.Action
	}{
		{"header value with CRLF", spec.Action{URL: "https://api.example.com/run", Headers: []spec.ActionParam{{Name: "X-A", Value: "${v}"}}}},
		{"header value with NUL", spec.Action{URL: "https://api.example.com/run", Headers: []spec.ActionParam{{Name: "X-A", Value: "a\x00b"}}}},
		{"JSON body that is not JSON after expansion", spec.Action{URL: "https://api.example.com/run", Body: `{"n":${word}}`}},
		{"unknown reference", spec.Action{URL: "https://api.example.com/${nope}"}},
		{"declared variable with no value", spec.Action{URL: "https://api.example.com/${empty}"}},
		{"missing numeric value", spec.Action{URL: "https://api.example.com/${__value.numeric}"}},
		{"missing data field", spec.Action{URL: "https://api.example.com/${__data.fields.nope}"}},
		{"unknown format", spec.Action{URL: "https://api.example.com/${word:nosuch}"}},
		{"invalid date format", spec.Action{URL: "https://api.example.com/${__from:csv}"}},
		{"reference that builds the host", spec.Action{URL: "https://${host}.example.com/run"}},
		{"user info in the template", spec.Action{URL: "https://api.example.com@${host}/run"}},
		{"unterminated reference", spec.Action{URL: "https://api.example.com/${word"}},
		{"unsupported content type", spec.Action{URL: "https://api.example.com/run", ContentType: "text/html", Body: "x"}},
		{"address that does not parse", spec.Action{URL: "https://%zz/run"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := spec.ExpandAction(tc.action, env); err == nil {
				t.Fatalf("expanded to %+v, want an error", got)
			}
		})
	}
}

func TestExpandActionErrorsNameReferencesNeverValues(t *testing.T) {
	env := spec.ActionEnv{Variables: map[string][]string{"secret": {"hunter2"}}}
	_, err := spec.ExpandAction(spec.Action{
		URL:     "https://api.example.com/run",
		Headers: []spec.ActionParam{{Name: "X-A", Value: "${secret}\r\n"}},
	}, env)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error repeats a value: %v", err)
	}
}
