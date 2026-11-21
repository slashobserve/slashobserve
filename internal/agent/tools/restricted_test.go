package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

func restrictedTools(t *testing.T) *Registry {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	answers := &fakeDS{
		rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
			return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
		},
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
		},
	}
	reg, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: answers})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.AddAdminsOnly(datasource.Instance{UID: "audit", Name: "Audit", Datasource: answers}); err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(reg, logger)
	access := folders.NewAccess(database.DB)
	return New(Deps{Datasources: reg, Runner: runner, Dashboards: dashboards.New(database.DB, runner, access, logger), Folders: access, Logger: logger})
}

func TestASaveOnAnAdminsOnlyDatasourceOutsideAnAdminsOnlyFolderAnswersRestrictedContent(t *testing.T) {
	r := restrictedTools(t)
	panel := strings.Replace(statPanel("p_audit", 0), `"datasource":"`+fakeUID+`"`, `"datasource":"audit"`, 1)
	input := fmt.Sprintf(`{"spec":{"specVersion":"0","title":"Salaries","time":{"from":"now-15m","to":"now"},"panels":[%s]}}`, panel)

	res, err := r.Call(auth.WithPrincipal(t.Context(), adminCaller), nameCreateDashboard, []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	got := textOf(t, res)
	if !res.IsError || !strings.Contains(got, `"code":"restricted_content"`) || !strings.Contains(got, `datasource \"audit\"`) ||
		!strings.Contains(got, "folder only Admins can read") || !strings.Contains(got, "admins_only is true (see list_folders)") ||
		!strings.Contains(got, "observe folders permissions admins-only") {
		t.Fatalf("create_dashboard = %s, want restricted_content naming the datasource", got)
	}
}
