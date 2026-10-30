package proxyhttp

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestParseQueryRefusesForbiddenNamesWhateverTheirValue(t *testing.T) {
	for _, raw := range []string{
		"query=up",
		"expr=up",
		"sql=select+1",
		"q=up",
		"query=%zz",
		"expr=%",
		"sql=a%zz&from=now-1h",
		"from=%zz&query=up",
		"%71=up",
		"%71uery=%zz",
		"q",
		"q=up;x=1",
		"query=up&query=down",
		"var-a=1&q=",
	} {
		if _, err := parseQuery(raw); !errors.Is(err, errQueryNotInSpec) {
			t.Errorf("parseQuery(%q) = %v, want errQueryNotInSpec", raw, err)
		}
	}
}

func TestParseQueryRejectsMalformedQueryStringsAsInvalidRequests(t *testing.T) {
	for _, raw := range []string{
		"from=%zz",
		"to=%",
		"var-a=%zz",
		"%zz=1",
		"max_data_points=%1",
		"from=now-1h;to=now",
		"a=b;c",
	} {
		_, err := parseQuery(raw)
		if !errors.Is(err, queryproxy.ErrInvalidRequest) {
			t.Errorf("parseQuery(%q) = %v, want ErrInvalidRequest", raw, err)
		}
		if err != nil && strings.Contains(err.Error(), "%") {
			t.Errorf("parseQuery(%q) echoes the raw input: %v", raw, err)
		}
	}
}

func TestParseQueryAcceptsWellFormedStrings(t *testing.T) {
	q, err := parseQuery("from=now-6h&var-pod=a%20b&var-pod=c&flag")
	if err != nil {
		t.Fatal(err)
	}
	if got := q["var-pod"]; !reflect.DeepEqual(got, []string{"a b", "c"}) {
		t.Fatalf("var-pod = %v", got)
	}
	if _, err := parseQuery(""); err != nil {
		t.Fatalf("empty query string: %v", err)
	}
}

func parse(t *testing.T, raw string, allowMaxDataPoints bool) (params, error) {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("test query %q: %v", raw, err)
	}
	return parseParams(q, allowMaxDataPoints)
}

func TestParseParamsDefaults(t *testing.T) {
	p, err := parse(t, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.maxDataPoints != defaultMaxDataPoints || p.from != "" || p.to != "" || len(p.selections) != 0 {
		t.Fatalf("params = %+v", p)
	}
}

func TestParseParamsSelections(t *testing.T) {
	p, err := parse(t, "var-pod=a&var-pod=b&var-env=%24__all&var-both=%24__all&var-both=x", true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]queryproxy.Selection{
		"pod":  {Values: []string{"a", "b"}},
		"env":  {All: true},
		"both": {All: true, Values: []string{"x"}},
	}
	if !reflect.DeepEqual(p.selections, want) {
		t.Fatalf("selections = %+v, want %+v", p.selections, want)
	}
}

func TestParseParamsRejectsUnknownAndMalformedParameters(t *testing.T) {
	for _, raw := range []string{
		"whatever=1",
		"var-=1",
		"from=now-1h&from=now-2h",
		"to=now&to=now",
		"from=",
		"to=",
		"from=" + strings.Repeat("a", maxTimeParamBytes+1),
		"VAR-a=1",
	} {
		if _, err := parse(t, raw, true); !errors.Is(err, queryproxy.ErrInvalidRequest) {
			t.Errorf("parseParams(%q) = %v, want ErrInvalidRequest", raw, err)
		}
	}
}

func TestParseParamsMaxDataPointsBounds(t *testing.T) {
	for raw, want := range map[string]int{"10": 10, "11000": 11000, "1000": 1000} {
		p, err := parse(t, "max_data_points="+raw, true)
		if err != nil || p.maxDataPoints != want {
			t.Errorf("max_data_points=%s: %+v, %v", raw, p, err)
		}
	}
	for _, raw := range []string{"9", "11001", "0", "-10", "abc", "1e3", "+10", "010", "%2010", "10.0", "", "99999999999999999999"} {
		if _, err := parse(t, "max_data_points="+raw, true); !errors.Is(err, queryproxy.ErrInvalidRequest) {
			t.Errorf("max_data_points=%q accepted: %v", raw, err)
		}
	}
	if _, err := parse(t, "max_data_points=10&max_data_points=20", true); !errors.Is(err, queryproxy.ErrInvalidRequest) {
		t.Errorf("repeated max_data_points accepted: %v", err)
	}
}

func TestParseParamsTimezone(t *testing.T) {
	p, err := parse(t, "tz=Asia/Kolkata", true)
	if err != nil || p.timezone != "Asia/Kolkata" {
		t.Fatalf("tz=Asia/Kolkata: %+v, %v", p, err)
	}
	for _, raw := range []string{"tz=", "tz=Mars/Base", "tz=Local", "tz=Asia/Kolkata&tz=utc", "tz=../etc/passwd"} {
		_, err := parse(t, raw, true)
		if !errors.Is(err, queryproxy.ErrInvalidRequest) || !strings.Contains(err.Error(), "tz is not a known time zone") && !strings.Contains(err.Error(), "given once") {
			t.Errorf("parseParams(%q) = %v, want an invalid tz", raw, err)
		}
	}
}

func TestParseParamsRefusesMaxDataPointsWhereItIsNotAccepted(t *testing.T) {
	if _, err := parse(t, "max_data_points=100", false); !errors.Is(err, queryproxy.ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestTimeRange(t *testing.T) {
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	d := &spec.Dashboard{Time: spec.TimeRange{From: "now-1h", To: "now"}}

	p, _ := parse(t, "", true)
	rng, _, err := p.timeRange(d, now)
	if err != nil || !rng.To.Equal(now) || rng.To.Sub(rng.From) != time.Hour {
		t.Fatalf("default range = %+v, %v", rng, err)
	}

	p, _ = parse(t, "from=now-2h&to=now-1h", true)
	rng, _, err = p.timeRange(d, now)
	if err != nil || !rng.From.Equal(now.Add(-2*time.Hour)) || !rng.To.Equal(now.Add(-time.Hour)) {
		t.Fatalf("explicit range = %+v, %v", rng, err)
	}

	for _, raw := range []string{"from=yesterday", "to=tomorrow-ish", "from=now&to=now-1h", "from=now-401d", "from=now&to=now"} {
		p, _ := parse(t, raw, true)
		if _, _, err := p.timeRange(d, now); !errors.Is(err, queryproxy.ErrInvalidRequest) {
			t.Errorf("%q: err = %v, want ErrInvalidRequest", raw, err)
		}
	}
}

func TestTimeRangeResolvesInTheDashboardZone(t *testing.T) {
	now := time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		timezone  string
		query     string
		wantStart time.Time
	}{
		{"a named zone rounds at its local midnight", "Asia/Kolkata", "", time.Date(2026, 10, 30, 18, 30, 0, 0, time.UTC)},
		{"utc rounds at utc midnight", "utc", "", time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)},
		{"browser uses the tz parameter", "browser", "tz=America/New_York", time.Date(2026, 10, 30, 4, 0, 0, 0, time.UTC)},
		{"browser without tz falls back to utc", "browser", "", time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)},
		{"a named zone ignores tz", "Asia/Kolkata", "tz=America/New_York", time.Date(2026, 10, 30, 18, 30, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &spec.Dashboard{Timezone: tt.timezone, Time: spec.TimeRange{From: "now/d", To: "now"}}
			p, err := parse(t, tt.query, true)
			if err != nil {
				t.Fatal(err)
			}
			rng, _, err := p.timeRange(d, now)
			if err != nil || !rng.From.Equal(tt.wantStart) || !rng.To.Equal(now) {
				t.Fatalf("range = %+v, %v, want from %v to %v", rng, err, tt.wantStart, now)
			}
		})
	}
}

func TestTimeRangeAppliesTheNowDelay(t *testing.T) {
	now := time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)
	d := &spec.Dashboard{NowDelay: "1m", Time: spec.TimeRange{From: "now-1h", To: "now"}}
	p, _ := parse(t, "", true)
	rng, opts, err := p.timeRange(d, now)
	if err != nil || !rng.To.Equal(now.Add(-time.Minute)) || rng.To.Sub(rng.From) != time.Hour {
		t.Fatalf("range = %+v, %v", rng, err)
	}
	panelRng, err := panelTimeRange(&spec.Panel{TimeFrom: "now-30m"}, rng, now, opts)
	if err != nil || !panelRng.To.Equal(now.Add(-time.Minute)) || panelRng.To.Sub(panelRng.From) != 30*time.Minute {
		t.Fatalf("panel range = %+v, %v", panelRng, err)
	}
}

func TestPanelTimeRangeRejectsLongerThanFourHundredDays(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	dashboard := datasource.TimeRange{From: now.Add(-6 * time.Hour), To: now}
	tests := []struct {
		name      string
		timeFrom  string
		wantStart time.Time
		wantErr   bool
	}{
		{"at the limit", "now-400d", now.Add(-400 * 24 * time.Hour), false},
		{"past the limit", "now-401d", time.Time{}, true},
		{"no override", "", dashboard.From, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := panelTimeRange(&spec.Panel{TimeFrom: tt.timeFrom}, dashboard, now, spec.TimeOptions{})
			if tt.wantErr {
				if !errors.Is(err, queryproxy.ErrInvalidRequest) {
					t.Fatalf("err = %v, want ErrInvalidRequest", err)
				}
				return
			}
			if err != nil || !got.From.Equal(tt.wantStart) {
				t.Fatalf("range = %v, err = %v, want from %v", got, err, tt.wantStart)
			}
		})
	}
}
