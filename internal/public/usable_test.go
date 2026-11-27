package public

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func parsed(t *testing.T, text string) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestALinkIsUsableOnlyWhenThePublicViewWouldOpenIt(t *testing.T) {
	registry, err := datasource.NewRegistry(slog.New(slog.DiscardHandler), datasource.Instance{UID: "prom", Datasource: typed{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(datasource.Instance{UID: "audit", Datasource: typed{}}); err != nil {
		t.Fatal(err)
	}
	s := &Service{registry: registry}
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Hour)
	plain := parsed(t, plainSpec)
	scoped := parsed(t, userSpec)
	audit := parsed(t, strings.Replace(plainSpec, `"datasource":"prom"`, `"datasource":"audit"`, 1))
	cases := []struct {
		name       string
		expiresAt  *time.Time
		adminsOnly bool
		latest     *spec.Dashboard
		want       string
	}{
		{"no expiry", nil, false, plain, ""},
		{"expires later", &future, false, plain, ""},
		{"expired", &past, false, plain, reasonExpired},
		{"expires right now", &now, false, plain, reasonExpired},
		{"in a folder only Admins can read", nil, true, plain, reasonAdminsOnlyFolder},
		{"reads the viewer's identity", nil, false, scoped, reasonUserScoped},
		{"reads a datasource only Admins can query", nil, false, audit, reasonRestrictedDatasource},
		{"expired and user scoped", &past, false, scoped, reasonExpired},
		{"expired in a folder only Admins can read", &past, true, audit, reasonExpired},
		{"a restricted datasource in a folder only Admins can read", nil, true, audit, reasonAdminsOnlyFolder},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link := judged(Link{ExpiresAt: tc.expiresAt}, tc.adminsOnly, s.dashboardReason(tc.latest), now)
			if link.UnusableReason != tc.want || link.Usable != (tc.want == "") || link.Expired != (tc.want == reasonExpired) {
				t.Fatalf("link = usable %v, expired %v, reason %q; want reason %q", link.Usable, link.Expired, link.UnusableReason, tc.want)
			}
			if tc.want != "" && !slices.Contains(UnusableReasons, tc.want) {
				t.Fatalf("UnusableReasons lacks %q", tc.want)
			}
		})
	}
}
