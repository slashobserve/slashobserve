package spec_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

type resolveCase struct {
	Expr    string `json:"expr"`
	Now     string `json:"now"`
	RoundUp bool   `json:"roundUp"`
	Options struct {
		Timezone             string         `json:"timezone"`
		WeekStart            spec.WeekStart `json:"weekStart"`
		FiscalYearStartMonth int            `json:"fiscalYearStartMonth"`
		NowDelay             string         `json:"nowDelay"`
	} `json:"options"`
	Want string `json:"want"`
}

func TestResolveTimeSharedCases(t *testing.T) {
	var cases []resolveCase
	if err := json.Unmarshal(readExample(t, "time/resolve-cases.json"), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 40 {
		t.Fatalf("%d cases, want at least 40", len(cases))
	}
	for _, c := range cases {
		d := &spec.Dashboard{
			Timezone:             c.Options.Timezone,
			WeekStart:            c.Options.WeekStart,
			FiscalYearStartMonth: c.Options.FiscalYearStartMonth,
			NowDelay:             c.Options.NowDelay,
		}
		opts, err := spec.TimeOptionsFor(d, "")
		if err != nil {
			t.Fatalf("%s: %v", c.Expr, err)
		}
		got, err := spec.ResolveTime(c.Expr, mustTime(t, c.Now), c.RoundUp, opts)
		if err != nil {
			t.Fatalf("%s at %s: %v", c.Expr, c.Now, err)
		}
		if want := mustTime(t, c.Want); !got.Equal(want) {
			t.Errorf("%s at %s (roundUp=%v, %+v): got %s, want %s", c.Expr, c.Now, c.RoundUp, c.Options, got.Format(time.RFC3339Nano), c.Want)
		}
	}
}

func TestTimeOptionsFor(t *testing.T) {
	cases := []struct {
		name        string
		dashboard   spec.Dashboard
		browserZone string
		wantZone    string
		wantDelay   time.Duration
		wantErr     bool
	}{
		{"no timezone is utc", spec.Dashboard{}, "", "UTC", 0, false},
		{"no timezone ignores the browser zone", spec.Dashboard{}, "Asia/Kolkata", "UTC", 0, false},
		{"utc", spec.Dashboard{Timezone: "utc"}, "Asia/Kolkata", "UTC", 0, false},
		{"a named zone", spec.Dashboard{Timezone: "America/New_York"}, "Asia/Kolkata", "America/New_York", 0, false},
		{"a zone without a slash", spec.Dashboard{Timezone: "EST"}, "", "EST", 0, false},
		{"browser uses the browser zone", spec.Dashboard{Timezone: "browser"}, "Asia/Kolkata", "Asia/Kolkata", 0, false},
		{"browser without a zone is utc", spec.Dashboard{Timezone: "browser"}, "", "UTC", 0, false},
		{"browser with an unknown zone", spec.Dashboard{Timezone: "browser"}, "Mars/Base", "", 0, true},
		{"an unknown zone", spec.Dashboard{Timezone: "Mars/Base"}, "", "", 0, true},
		{"Local is not a zone", spec.Dashboard{Timezone: "Local"}, "", "", 0, true},
		{"seconds of delay", spec.Dashboard{NowDelay: "30s"}, "", "UTC", 30 * time.Second, false},
		{"hours of delay", spec.Dashboard{NowDelay: "2h"}, "", "UTC", 2 * time.Hour, false},
		{"a malformed delay", spec.Dashboard{NowDelay: "soon"}, "", "", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := spec.TimeOptionsFor(&c.dashboard, c.browserZone)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if got.Location.String() != c.wantZone || got.NowDelay != c.wantDelay {
				t.Fatalf("got zone %s delay %s, want zone %s delay %s", got.Location, got.NowDelay, c.wantZone, c.wantDelay)
			}
		})
	}
}

func TestTimeOptionsForCarriesWeekAndFiscalSettings(t *testing.T) {
	got, err := spec.TimeOptionsFor(&spec.Dashboard{WeekStart: spec.WeekStartSunday, FiscalYearStartMonth: 3}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.WeekStart != spec.WeekStartSunday || got.FiscalYearStartMonth != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseChecksTheTimezoneIsKnown(t *testing.T) {
	base := string(readExample(t, "minimal.json"))
	withZone := func(zone string) []byte {
		return []byte(base[:len(base)-2] + `, "timezone": "` + zone + `"}`)
	}
	for _, zone := range []string{"browser", "utc", "Asia/Kolkata", "Etc/GMT+5", "America/Argentina/Buenos_Aires", "EST"} {
		if _, err := spec.Parse(withZone(zone)); err != nil {
			t.Errorf("timezone %q: %v", zone, err)
		}
	}
	for _, zone := range []string{"Mars/Base", "Local", "Nowhere"} {
		issues := parseIssues(t, withZone(zone))
		if len(issues) != 1 || issues[0].Code != spec.CodeUnknownTimezone || issues[0].Path != "/timezone" {
			t.Errorf("timezone %q: issues = %+v", zone, issues)
		}
	}
	for _, zone := range []string{"", "Asia/", "a b", "/UTC", "Asia/Kolkata/a/b"} {
		issues := parseIssues(t, withZone(zone))
		if len(issues) != 1 || issues[0].Code != spec.CodeSchema || issues[0].Path != "/timezone" {
			t.Errorf("timezone %q: issues = %+v", zone, issues)
		}
	}
}
