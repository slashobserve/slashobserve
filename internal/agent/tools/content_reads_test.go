package tools

import (
	"maps"
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
	readsContextSrc inRunRead = "a context source, through datasource.Access.Reads and NoteRead"

	readsOneObject  inRunRead = "one stored object's content, noted through folders.Read of the dashboard or observation it belongs to"
	readsKeptTitles inRunRead = "titles only, kept or left out through restrictedTitles"
	readsPrivate    inRunRead = "chats, runs and the inbox, through guardPrivateRead"
	readsAudit      inRunRead = "the audit trail, through guardAuditRead"
	writesInScope   inRunRead = "only what its write scope lets it change: the run's subject, what it created, the build it committed, runs of its chat or the requester's own state"
)

var inRunReads = map[string]inRunRead{
	nameCompareTraces:      readsDatasource,
	nameGetTrace:           readsDatasource,
	nameListLabelValues:    readsDatasource,
	nameListLabels:         readsDatasource,
	nameListMetrics:        readsDatasource,
	nameListServices:       readsDatasource,
	nameLogContext:         readsDatasource,
	nameLogFacets:          readsDatasource,
	nameLogPatterns:        readsDatasource,
	nameLogsForTrace:       readsDatasource,
	nameQueryInstant:       readsDatasource,
	nameQueryRange:         readsDatasource,
	nameSampleRows:         readsDatasource,
	nameTraceStats:         readsDatasource,
	nameListDatasources:    readsDatasource,
	nameLintQuery:          readsDatasource,
	nameGetSchema:          readsDatasource,
	nameGetDashboard:       readsContent,
	nameGetPanels:          readsContent,
	nameGetObservation:     readsContent,
	nameListObservations:   readsTitles,
	nameListAnnotations:    readsContent,
	nameListDashboards:     readsTitles,
	nameListFolders:        readsTitles,
	nameCreateDashboard:    readsRunOnly,
	nameUpdateDashboard:    readsRunOnly,
	nameUpdatePanels:       readsRunOnly,
	nameImportGrafana:      readsRunOnly,
	nameSaveFinding:        readsRunOnly,
	nameGetSpecGuide:       readsNothing,
	nameListContextSources: readsContextSrc,
	nameCallContextTool:    readsContextSrc,
	nameFindContextTools:   readsContextSrc,

	nameRecheckDatasource:     readsDatasource,
	nameListDashboardVersions: readsContent,
	nameDiffDashboardVersions: readsContent,
	nameListDashboardViewers:  readsContent,
	nameListComments:          readsContent,
	nameListMentionable:       readsContent,
	nameListRCAs:              readsContent,
	nameListProposals:         readsContent,
	nameGetRCA:                readsOneObject,
	nameGetProposal:           readsOneObject,

	nameListPinnedDashboards: readsKeptTitles,
	nameGetHomeDashboard:     readsKeptTitles,
	nameListDashboardHealth:  readsKeptTitles,
	nameSearchWorkspace:      readsKeptTitles,
	nameListSavedSearches:    readsKeptTitles,
	nameListActivity:         readsKeptTitles,
	nameListAccessRequests:   readsKeptTitles,

	nameGetRun:      readsPrivate,
	nameGetRunBuild: readsPrivate,
	nameListRuns:    readsPrivate,
	nameListChats:   readsPrivate,
	nameGetChat:     readsPrivate,
	nameListInbox:   readsPrivate,

	nameSearchAuditEvents:    readsAudit,
	nameGetAuditEvent:        readsAudit,
	nameListAuditFacets:      readsAudit,
	nameGetAuditHistogram:    readsAudit,
	nameAggregateAuditEvents: readsAudit,
	nameListAuditFields:      readsAudit,
}

func TestEveryToolOfferedInARunSaysHowItReadsWhatOnlyAdminsRead(t *testing.T) {
	reads := maps.Clone(inRunReads)
	for name := range writeTargets {
		if kind, ok := reads[name]; ok {
			t.Errorf("%s has a write scope and inRunReads says it reads %s", name, kind)
		}
		reads[name] = writesInScope
	}
	offered := map[string]bool{}
	for _, tool := range New(Deps{}).List() {
		if tool.MCPOnly {
			continue
		}
		offered[tool.Name] = true
		if _, ok := reads[tool.Name]; !ok {
			t.Errorf("%s is offered in runs but inRunReads does not say how it reads what only Admins read", tool.Name)
		}
		if reads[tool.Name] == readsContent && contentReads[tool.Name] == nil {
			t.Errorf("%s reads stored content in runs but has no case in contentReads", tool.Name)
		}
	}
	for name := range reads {
		if !offered[name] {
			t.Errorf("inRunReads names %s, which runs are not offered", name)
		}
	}
}

const adminsOnlyWord = "Sign-ins-by-Root"

type adminsOnlyContent struct {
	folder, dashboard, observation pgtype.UUID
}

var contentReads = map[string]func(adminsOnlyContent) string{
	nameGetDashboard: func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
	nameGetPanels: func(c adminsOnlyContent) string {
		return `{"dashboard_id":"` + c.dashboard.String() + `","ids":["p_up"]}`
	},
	nameGetObservation:        func(c adminsOnlyContent) string { return `{"observation_id":"` + c.observation.String() + `"}` },
	nameListAnnotations:       func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
	nameListDashboardVersions: func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
	nameDiffDashboardVersions: func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
	nameListDashboardViewers:  func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
	nameListComments:          func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
	nameListMentionable:       func(c adminsOnlyContent) string { return `{"target":"dashboard:` + c.dashboard.String() + `"}` },
	nameListRCAs:              func(c adminsOnlyContent) string { return `{"observation_id":"` + c.observation.String() + `"}` },
	nameListProposals:         func(c adminsOnlyContent) string { return `{"dashboard_id":"` + c.dashboard.String() + `"}` },
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
	c := adminsOnlyContent{folder: folder, dashboard: mustUUID(t, dash.ID)}
	r.db.Exec(t, `INSERT INTO annotations (dashboard_id, time_from, text, tags) VALUES ($1, now() - interval '1 minute', $2, ARRAY[]::text[])`, c.dashboard, adminsOnlyWord+" annotation")
	if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `INSERT INTO observations (folder_id, title, summary, created_by_kind, created_by_id, created_by_name) VALUES ($1, $2, $2, 'user', $3::uuid, 'Root') RETURNING id`, folder, adminsOnlyWord+" observation", adminCaller.ID).Scan(&c.observation); err != nil {
		t.Fatal(err)
	}
	return c
}

func (r *accessRig) callInRun(t *testing.T, audience datasource.Audience, tool, input string) (Result, bool) {
	t.Helper()
	return r.callAs(t, Run{Mode: ModeInvestigate, Access: datasource.Access{By: adminCaller, Audience: audience}}, tool, input)
}

func (r *accessRig) callAs(t *testing.T, run Run, tool, input string) (Result, bool) {
	t.Helper()
	ctx, read := datasource.NotingRestrictedReads(auth.WithPrincipal(reading(t.Context()), adminCaller))
	run.RequestID = "00000000-0000-4000-8000-000000000001"
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

func TestAnAdminsRunListsTitlesInOtherAdminsOnlyFoldersOnlyWhenAskedAndIsMarkedForThem(t *testing.T) {
	r := newAccessRig(t)
	content := r.adminsOnlyContent(t)
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

	for _, name := range []string{nameListDashboards, nameListFolders, nameListObservations} {
		t.Run(name, func(t *testing.T) {
			shipsOne := name != nameListObservations
			for _, audience := range []datasource.Audience{datasource.AudienceCaller, datasource.AudienceAdmins} {
				res, marked := r.callInRun(t, audience, name, `{}`)
				if text := textOf(t, res); res.IsError || marked || strings.Contains(text, adminsOnlyWord) || shipsOne != strings.Contains(text, "Shipped audit") ||
					!strings.Contains(text, `"left_out":1`) || !strings.Contains(text, "include_restricted: true") {
					t.Errorf("%s by default: marked %v, read %s", audience, marked, text)
				}
				res, marked = r.callInRun(t, audience, name, `{"include_restricted":true}`)
				if text := textOf(t, res); res.IsError || !marked || !strings.Contains(text, adminsOnlyWord) || strings.Contains(text, "left_out") {
					t.Errorf("%s with include_restricted: marked %v, read %s", audience, marked, text)
				}
			}
			res, marked := r.callInRun(t, datasource.AudienceEveryone, name, `{"include_restricted":true}`)
			if text := textOf(t, res); !res.IsError || marked || strings.Contains(text, adminsOnlyWord) {
				t.Errorf("with include_restricted writing where everyone reads: marked %v, read %s", marked, text)
			}
			home := Run{Mode: ModeInvestigate, WriteFolders: []pgtype.UUID{content.folder}, Access: datasource.Access{By: adminCaller, Audience: datasource.AudienceAdmins}}
			res, marked = r.callAs(t, home, name, `{}`)
			if text := textOf(t, res); res.IsError || marked || !strings.Contains(text, adminsOnlyWord) || strings.Contains(text, "left_out") {
				t.Errorf("listing the folder the run writes to: marked %v, read %s", marked, text)
			}
		})
	}
}

func TestARunReadingWhatItIsAboutIsNotMarkedButReadingOtherContentOnlyAdminsReadIs(t *testing.T) {
	r := newAccessRig(t)
	content := r.adminsOnlyContent(t)
	elsewhere := "55555555-5555-4555-8555-555555555555"
	admin := datasource.Access{By: adminCaller, Audience: datasource.AudienceCaller}
	readDashboard := `{"dashboard_id":"` + content.dashboard.String() + `"}`
	readObservation := `{"observation_id":"` + content.observation.String() + `"}`
	for _, c := range []struct {
		name   string
		run    Run
		tool   string
		input  string
		marked bool
	}{
		{"a revision of the dashboard", Run{Mode: ModeRevise, DashboardID: content.dashboard.String(), Access: admin}, nameGetDashboard, readDashboard, false},
		{"a suggestion on the dashboard", Run{Mode: ModeSuggest, DashboardID: content.dashboard.String(), PanelID: "p_up", Access: admin}, nameGetDashboard, readDashboard, false},
		{"an ask about the dashboard", Run{Mode: ModeInvestigate, DashboardID: content.dashboard.String(), Access: admin}, nameGetDashboard, readDashboard, false},
		{"an investigation about the observation", Run{Mode: ModeInvestigate, Observations: []string{content.observation.String()}, Access: admin}, nameGetObservation, readObservation, false},
		{"an ask about another dashboard", Run{Mode: ModeInvestigate, DashboardID: elsewhere, Access: admin}, nameGetDashboard, readDashboard, true},
		{"an investigation about another observation", Run{Mode: ModeInvestigate, Observations: []string{elsewhere}, Access: admin}, nameGetObservation, readObservation, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, marked := r.callAs(t, c.run, c.tool, c.input)
			if res.IsError || marked != c.marked {
				t.Fatalf("marked %v, want %v, read %s", marked, c.marked, textOf(t, res))
			}
		})
	}
}
