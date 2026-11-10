package activity

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseFilterReadsEveryParameter(t *testing.T) {
	q := url.Values{
		"dashboard_id": {"11111111-1111-4111-8111-111111111111"},
		"folder_id":    {"22222222-2222-4222-8222-222222222222"},
		"type":         {"dashboard.moved", "run.failed"},
		"since":        {"2026-11-01T10:00:00+02:00"},
	}

	f, err := ParseFilter(q)

	if err != nil {
		t.Fatal(err)
	}
	if f.DashboardID.String() != "11111111-1111-4111-8111-111111111111" || f.FolderID.String() != "22222222-2222-4222-8222-222222222222" {
		t.Errorf("ids = %v, %v", f.DashboardID, f.FolderID)
	}
	if !slices.Equal(f.Types, []string{"dashboard.moved", "run.failed"}) {
		t.Errorf("types = %v", f.Types)
	}
	if want := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC); !f.Since.Equal(want) {
		t.Errorf("since = %v, want %v", f.Since, want)
	}
}

func TestParseFilterAcceptsNoParameters(t *testing.T) {
	f, err := ParseFilter(url.Values{})

	if err != nil {
		t.Fatal(err)
	}
	if f.DashboardID.Valid || f.FolderID.Valid || len(f.Types) != 0 || !f.Since.IsZero() {
		t.Fatalf("got %+v", f)
	}
}

func TestParseFilterRefusesBadParametersAndNamesThem(t *testing.T) {
	fifteen := make([]string, 15)
	for i := range fifteen {
		fifteen[i] = TypeRunQueued
	}
	tests := []struct {
		name  string
		query url.Values
		want  string
	}{
		{"dashboard id", url.Values{"dashboard_id": {"nope"}}, "dashboard_id"},
		{"folder id", url.Values{"folder_id": {"11111111111141118111111111111111"}}, "folder_id"},
		{"unknown type", url.Values{"type": {"dashboard.moved", "comment.created"}}, "comment.created"},
		{"too many types", url.Values{"type": fifteen}, "type"},
		{"since", url.Values{"since": {"yesterday"}}, "since"},
		{"since without a zone", url.Values{"since": {"2026-11-01T10:00:00"}}, "since"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFilter(tt.query)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error naming %q", err, tt.want)
			}
		})
	}
}

func TestParseFilterAcceptsAllFourteenTypes(t *testing.T) {
	f, err := ParseFilter(url.Values{"type": Types()})

	if err != nil {
		t.Fatal(err)
	}
	if len(f.Types) != 14 {
		t.Fatalf("types = %d", len(f.Types))
	}
}
