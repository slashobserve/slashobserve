package grafanaconn_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func signedInAs(p auth.Principal) func(*http.Request) (auth.Principal, bool, error) {
	return func(*http.Request) (auth.Principal, bool, error) { return p, true, nil }
}

func TestDashboardRedirectAnswersTheImportedDashboardsAddress(t *testing.T) {
	r := newRig(t)
	dashboardID := importedFromGrafana(t, r)
	redirect := r.svc.DashboardRedirect(signedInAs(r.viewer))
	req := httptest.NewRequest(http.MethodGet, "/d/checkout", nil)

	if got, ok := redirect(req, "checkout"); !ok || got != "/d/"+dashboardID {
		t.Fatalf("checkout = %q, %t; want /d/%s", got, ok, dashboardID)
	}
	if got, ok := redirect(req, "never-imported"); ok || got != "" {
		t.Fatalf("unknown uid = %q, %t; want no redirect", got, ok)
	}
}

func TestDashboardRedirectResolvesNothingForCallersWhoCannotSeeTheDashboard(t *testing.T) {
	r := newRig(t)
	dashboardID := importedFromGrafana(t, r)
	r.db.Exec(t, "INSERT INTO folder_permissions (folder_id, group_name, level) SELECT folder_id, 'secret-team', 'view' FROM dashboards WHERE id = $1", dashboardID)
	req := httptest.NewRequest(http.MethodGet, "/d/checkout", nil)

	callers := map[string]func(*http.Request) (auth.Principal, bool, error){
		"anonymous":                 func(*http.Request) (auth.Principal, bool, error) { return auth.Principal{}, false, nil },
		"session lookup fails":      func(*http.Request) (auth.Principal, bool, error) { return r.admin, false, errors.New("database down") },
		"viewer outside the folder": signedInAs(r.viewer),
	}
	for name, principalOf := range callers {
		if got, ok := r.svc.DashboardRedirect(principalOf)(req, "checkout"); ok || got != "" {
			t.Fatalf("%s = %q, %t; want no redirect", name, got, ok)
		}
	}
	if got, ok := r.svc.DashboardRedirect(signedInAs(r.admin))(req, "checkout"); !ok || got != "/d/"+dashboardID {
		t.Fatalf("the folder's creator = %q, %t; want /d/%s", got, ok, dashboardID)
	}
}
