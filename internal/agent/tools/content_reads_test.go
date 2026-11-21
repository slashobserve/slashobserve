package tools

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

type inRunRead string

const (
	readsDatasource inRunRead = "a datasource, through datasource.Registry.For"
	readsContent    inRunRead = "stored content, noted through folders.Read"
	readsTitles     inRunRead = "titles only, from folders.Access.Levels"
	readsRunOnly    inRunRead = "only the run's own build or input"
	readsNothing    inRunRead = "no stored content"
)

var inRunReads = map[string]inRunRead{
	nameCompareTraces:    readsDatasource,
	nameGetTrace:         readsDatasource,
	nameListLabelValues:  readsDatasource,
	nameListLabels:       readsDatasource,
	nameListMetrics:      readsDatasource,
	nameListServices:     readsDatasource,
	nameLogContext:       readsDatasource,
	nameLogFacets:        readsDatasource,
	nameLogPatterns:      readsDatasource,
	nameLogsForTrace:     readsDatasource,
	nameQueryInstant:     readsDatasource,
	nameQueryRange:       readsDatasource,
	nameSampleRows:       readsDatasource,
	nameTraceStats:       readsDatasource,
	nameListDatasources:  readsDatasource,
	nameLintQuery:        readsDatasource,
	nameGetSchema:        readsDatasource,
	nameGetDashboard:     readsContent,
	nameGetPanels:        readsContent,
	nameGetObservation:   readsContent,
	nameListObservations: readsContent,
	nameListAnnotations:  readsContent,
	nameListDashboards:   readsTitles,
	nameListFolders:      readsTitles,
	nameCreateDashboard:  readsRunOnly,
	nameUpdateDashboard:  readsRunOnly,
	nameUpdatePanels:     readsRunOnly,
	nameImportGrafana:    readsRunOnly,
	nameSaveFinding:      readsRunOnly,
	nameGetSpecGuide:     readsNothing,
}

func TestEveryToolOfferedInARunSaysHowItReadsWhatOnlyAdminsRead(t *testing.T) {
	offered := map[string]bool{}
	for _, tool := range New(Deps{}).List() {
		if tool.MCPOnly {
			continue
		}
		offered[tool.Name] = true
		if _, ok := inRunReads[tool.Name]; !ok {
			t.Errorf("%s is offered in runs but inRunReads does not say how it reads what only Admins read", tool.Name)
		}
		if inRunReads[tool.Name] == readsContent && contentReads[tool.Name] == nil {
			t.Errorf("%s reads stored content in runs but has no case in contentReads", tool.Name)
		}
	}
	for name := range inRunReads {
		if !offered[name] {
			t.Errorf("inRunReads names %s, which runs are not offered", name)
		}
	}
}

const adminsOnlyWord = "Sign-ins-by-Root"

type adminsOnlyContent struct {
	dashboard, observation pgtype.UUID
}

var contentReads = map[string]func(adminsOnlyContent) string{
	nameGetDashboard: func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
	nameGetPanels: func(c adminsOnlyContent) string {
		return `{"dashboard_id":"` + c.dashboard.String() + `","ids":["p_up"]}`
	},
	nameGetObservation:   func(c adminsOnlyContent) string { return `{"observation_id":"` + c.observation.String() + `"}` },
	nameListObservations: func(adminsOnlyContent) string { return `{}` },
	nameListAnnotations:  func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
}

func (r *accessRig) adminsOnlyContent(t *testing.T) adminsOnlyContent {
	t.Helper()
	var folder pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO folders (title, admins_only) VALUES ($1, true) RETURNING id`, adminsOnlyWord+" notes").Scan(&folder); err != nil {
		t.Fatal(err)
	}
	dash, _, err := r.service.Create(reading(t.Context()), adminCaller, folder, []byte(boardSpec(adminsOnlyWord+" board")), "create")
	if err != nil {
		t.Fatal(err)
	}
	c := adminsOnlyContent{dashboard: mustUUID(t, dash.ID)}
	r.db.Exec(t, `INSERT INTO annotations (dashboard_id, time_from, text, tags) VALUES ($1, now() - interval '1 minute', $2, ARRAY[]::text[])`, c.dashboard, adminsOnlyWord+" annotation")
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO observations (folder_id, title, summary, created_by_kind, created_by_id, created_by_name) VALUES ($1, $2, $2, 'user', $3::uuid, 'Root') RETURNING id`, folder, adminsOnlyWord+" observation", adminCaller.ID).Scan(&c.observation); err != nil {
		t.Fatal(err)
	}
	return c
}

func (r *accessRig) callInRun(t *testing.T, audience datasource.Audience, tool, input string) (Result, bool) {
	t.Helper()
	ctx, read := datasource.NotingRestrictedReads(auth.WithPrincipal(reading(t.Context()), adminCaller))
	run := Run{Mode: ModeInvestigate, RequestID: "00000000-0000-4000-8000-000000000001", Access: datasource.Access{By: adminCaller, Audience: audience}}
	res, err := r.registry.Call(WithRun(ctx, run), tool, []byte(input))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res, read()
}

func TestARunReadsWhatOnlyAdminsReadOnlyWhenNoOneElseReadsWhatItWritesAndIsMarkedForIt(t *testing.T) {
	r := newAccessRig(t)
	content := r.adminsOnlyContent(t)
	for name, kind := range inRunReads {
		if kind != readsContent && kind != readsTitles {
			continue
		}
		input := `{}`
		if c := contentReads[name]; c != nil {
			input = c(content)
		}
		t.Run(name, func(t *testing.T) {
			shared, marked := r.callInRun(t, datasource.AudienceEveryone, name, input)
			if text := textOf(t, shared); strings.Contains(text, adminsOnlyWord) || marked || strings.HasPrefix(name, "get_") && !shared.IsError {
				t.Errorf("writing where everyone reads: marked %v, read %s", marked, text)
			}
			private, marked := r.callInRun(t, datasource.AudienceCaller, name, input)
			if text := textOf(t, private); private.IsError || marked != (kind == readsContent) {
				t.Errorf("an Admin's private run: marked %v, want %v, read %s", marked, kind == readsContent, text)
			}
		})
	}
}

func (r *accessRig) builtIn(t *testing.T, key string, ref pgtype.UUID) {
	t.Helper()
	r.db.Exec(t, `INSERT INTO starter_content (key, ref) VALUES ($1, $2)`, key, ref)
}

func TestAnAdminsPrivateRunListsTitlesInAdminsOnlyFoldersOnlyWhenAskedAndIsMarkedForThem(t *testing.T) {
	r := newAccessRig(t)
	r.adminsOnlyContent(t)
	var shippedFolder pgtype.UUID
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO folders (title, admins_only) VALUES ('Shipped audit folder', true) RETURNING id`).Scan(&shippedFolder); err != nil {
		t.Fatal(err)
	}
	shipped, _, err := r.service.Create(reading(t.Context()), adminCaller, shippedFolder, []byte(boardSpec("Shipped audit board")), "create")
	if err != nil {
		t.Fatal(err)
	}
	r.builtIn(t, "audit/folder", shippedFolder)
	r.builtIn(t, "audit/dashboards/shipped", mustUUID(t, shipped.ID))

	for _, name := range []string{nameListDashboards, nameListFolders} {
		t.Run(name, func(t *testing.T) {
			res, marked := r.callInRun(t, datasource.AudienceCaller, name, `{}`)
			if text := textOf(t, res); res.IsError || marked || strings.Contains(text, adminsOnlyWord) || !strings.Contains(text, "Shipped audit") ||
				!strings.Contains(text, `"left_out":1`) || !strings.Contains(text, "include_restricted: true") {
				t.Errorf("by default: marked %v, read %s", marked, text)
			}
			res, marked = r.callInRun(t, datasource.AudienceCaller, name, `{"include_restricted":true}`)
			if text := textOf(t, res); res.IsError || !marked || !strings.Contains(text, adminsOnlyWord) || strings.Contains(text, "left_out") {
				t.Errorf("with include_restricted: marked %v, read %s", marked, text)
			}
			res, marked = r.callInRun(t, datasource.AudienceEveryone, name, `{"include_restricted":true}`)
			if text := textOf(t, res); !res.IsError || marked || strings.Contains(text, adminsOnlyWord) {
				t.Errorf("with include_restricted writing where everyone reads: marked %v, read %s", marked, text)
			}
			res, marked = r.callInRun(t, datasource.AudienceAdmins, name, `{}`)
			if text := textOf(t, res); res.IsError || marked || !strings.Contains(text, adminsOnlyWord) {
				t.Errorf("writing where only Admins read: marked %v, read %s", marked, text)
			}
		})
	}
}

func TestARunReadingTheDashboardItChangesIsNotMarkedButAnAskAboutItIs(t *testing.T) {
	r := newAccessRig(t)
	content := r.adminsOnlyContent(t)
	for _, c := range []struct {
		mode   Mode
		marked bool
	}{{ModeRevise, false}, {ModeSuggest, false}, {ModeInvestigate, true}} {
		t.Run(string(c.mode), func(t *testing.T) {
			ctx, read := datasource.NotingRestrictedReads(auth.WithPrincipal(reading(t.Context()), adminCaller))
			run := Run{Mode: c.mode, RequestID: "00000000-0000-4000-8000-000000000001", DashboardID: content.dashboard.String(), PanelID: "p_up",
				Access: datasource.Access{By: adminCaller, Audience: datasource.AudienceCaller}}
			res, err := r.registry.Call(WithRun(ctx, run), nameGetDashboard, []byte(`{"dashboard_id":"`+content.dashboard.String()+`"}`))
			if err != nil {
				t.Fatal(err)
			}
			if read() != c.marked {
				t.Fatalf("marked %v, want %v, read %s", read(), c.marked, textOf(t, res))
			}
		})
	}
}
