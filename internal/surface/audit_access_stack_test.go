package surface

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/audit/trail/trailtest"
	"github.com/slashobserve/slashobserve/internal/auditfolder"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
	"github.com/slashobserve/slashobserve/internal/datasourceadmin"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/discoveryhttp"
	"github.com/slashobserve/slashobserve/internal/drafthttp"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/events"
	"github.com/slashobserve/slashobserve/internal/events/stream"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
	"github.com/slashobserve/slashobserve/internal/provision"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/queryproxy/proxyhttp"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/sealed"
	"github.com/slashobserve/slashobserve/internal/search"
	"github.com/slashobserve/slashobserve/internal/server"
	"github.com/slashobserve/slashobserve/internal/server/mcp"
	"github.com/slashobserve/slashobserve/internal/tailhttp"
	"github.com/slashobserve/slashobserve/internal/threads"
)

const (
	auditUID          = auditsource.UID
	controlUID        = "prom"
	marker            = "audit-access-marker"
	controlFolderName = "Team access control"
	controlBoard      = "Control audit access board"
	sessionCookie     = "so_session"
)

var roles = []auth.Role{auth.RoleAdmin, auth.RoleEditor, auth.RoleCommenter, auth.RoleViewer}

type credentialKind int

const (
	anonymousCaller credentialKind = iota
	personSession
	serviceAccountKey
)

type caller struct {
	name string
	kind credentialKind
	role auth.Role
}

func (c caller) admin() bool { return c.kind != anonymousCaller && c.role == auth.RoleAdmin }

func callers() []caller {
	out := []caller{{name: "Anonymous", kind: anonymousCaller}}
	for _, role := range roles {
		out = append(out, caller{name: title(role), kind: personSession, role: role})
	}
	for _, role := range roles {
		out = append(out, caller{name: title(role) + " key", kind: serviceAccountKey, role: role})
	}
	return out
}

func title(role auth.Role) string {
	s := string(role)
	return strings.ToUpper(s[:1]) + s[1:]
}

type stack struct {
	t              *testing.T
	server         *httptest.Server
	sessions       map[auth.Role]string
	keys           map[auth.Role]string
	personTokens   map[string]string
	auditFolder    string
	starters       []string
	starterTitle   string
	starterPanel   string
	controlBoard   string
	controlDir     string
	eventID        string
	adminID        string
	restrictedRun  string
	plainRun       string
	restrictedChat string
}

type mcpCredentials struct {
	keys   *auth.Service
	people map[string]string
}

func (m mcpCredentials) Authenticate(ctx context.Context, header string) (mcp.Credential, bool, error) {
	if p, ok, err := m.keys.BearerPrincipal(ctx, header); err != nil || ok {
		return mcp.Credential{Principal: p}, ok, err
	}
	id, ok := m.people[header]
	if !ok {
		return mcp.Credential{}, false, nil
	}
	p, err := m.keys.Principal(ctx, auth.KindUser, id)
	if err != nil {
		return mcp.Credential{}, false, err
	}
	return mcp.Credential{Principal: p, OAuth: true}, true, nil
}

func newStack(t *testing.T) *stack {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	database := dbtest.New(t)

	publicURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.New(auth.Config{PublicURL: publicURL, IssuerURL: "http://issuer.example/dex"}, database.Queries(), logger)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: controlUID, Datasource: datasource.NewUnavailable("prometheus", datasource.ErrUnavailable)})
	if err != nil {
		t.Fatal(err)
	}
	slots := queryproxy.NewSlots(map[string]int{controlUID: 8}, nil)
	reader := trail.NewReader(database.DB, logger)
	instance, err := auditsource.Instance(reader, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.AddBuiltIn(instance); err != nil {
		t.Fatal(err)
	}
	slots.SetTotal(auditsource.UID, auditsource.MaxConcurrentQueries)

	folderAccess := folders.NewAccess(database.DB)
	runner := queryproxy.NewRunner(registry, logger, queryproxy.WithSlots(slots))
	dashboardsService := dashboards.New(database.DB, runner, folderAccess, logger)
	proxyRunner := queryproxy.NewRunner(registry, logger, queryproxy.WithSlots(slots), queryproxy.WithValueCache())
	proxyHandler := proxyhttp.NewHandler(proxyRunner, dashboardsService, dashboardsService, folderAccess, registry, logger)
	activityStore := activity.NewStore(database.DB, folderAccess)
	searchService := search.New(database.DB, folderAccess, publicURL)
	revisionsService := revisions.NewService(database.DB, logger,
		revisions.WithAccess(folderAccess),
		revisions.WithPrincipals(authService),
		revisions.WithThreads(threads.NewRuns(database.DB)),
		revisions.WithBuiltInAgent(),
	)
	observeOptions := revisions.ObserveOptions{
		DashboardURL:       func(string, string) string { return "" },
		ObservationURL:     func(string) string { return "" },
		ObservationVisible: func(context.Context, auth.Principal, pgtype.UUID) (bool, error) { return false, nil },
	}
	toolRegistry := tools.New(tools.Deps{
		Runs:        revisions.NewRunDesk(revisionsService, observeOptions),
		Datasources: registry,
		Runner:      runner,
		Dashboards:  dashboardsService,
		Folders:     folderAccess,
		Activity:    activityStore,
		Audit:       reader,
		Search:      searchService,
		PublicURL:   publicURL,
		Logger:      logger,
	})
	broker := events.NewBroker(database.Listen, logger)
	brokerCtx, stopBroker := context.WithCancel(context.Background())
	brokerDone := make(chan struct{})
	go func() {
		defer close(brokerDone)
		broker.Run(brokerCtx)
	}()
	t.Cleanup(func() {
		stopBroker()
		<-brokerDone
		broker.Close()
	})
	datasourceAdmin := datasourceadmin.New(datasourceadmin.Deps{
		DB: database.DB, Registry: registry, Slots: slots, Key: sealed.Key(strings.Repeat("k", 32)),
		Domains: allowlist.New(database.DB, allowlist.Lock{}, logger), Access: folderAccess, Logger: logger,
	})

	s := &stack{t: t, sessions: map[auth.Role]string{}, keys: map[auth.Role]string{}, personTokens: map[string]string{}}
	s.seedPeople(database)
	s.seedAuditContent(database, dashboardsService, folderAccess, revisionsService, logger)

	handler, err := server.New(server.Options{
		Logger:          logger,
		Auth:            authService,
		Datasources:     registry,
		DatasourceUsage: dashboardsService,
		API: []server.Registrar{
			dashboardsService,
			folders.NewHandler(database.DB, folderAccess, logger),
			revisionsService,
			revisions.NewObserveHandler(revisionsService, observeOptions),
			trail.NewHandler(reader, authService, logger),
			stream.NewAuditHandler(broker, reader, authService, logger),
			activity.NewHandler(activityStore, logger),
			search.NewHandler(searchService, logger),
			proxyHandler,
			drafthttp.NewHandler(toolRegistry, logger),
			discoveryhttp.NewHandler(toolRegistry, logger),
			tailhttp.NewHandler(registry, proxyRunner, authService, logger),
			datasourceadmin.NewHandler(datasourceAdmin, logger),
		},
		Mounts: []server.Mount{{Path: "/mcp", Handler: mcp.NewHandler(toolRegistry, mcpCredentials{keys: authService, people: s.personTokens}, mcp.Options{PublicURL: publicURL, Version: "test", Logger: logger})}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.server = httptest.NewServer(handler)
	t.Cleanup(s.server.Close)
	return s
}

func (s *stack) seedPeople(database *dbtest.Database) {
	t := s.t
	ctx := t.Context()
	for _, role := range roles {
		user, err := database.Queries().UpsertUser(ctx, dbsqlc.UpsertUserParams{OidcIssuer: "i", OidcSubject: string(role), Email: string(role) + "@example.com", Name: title(role)})
		if err != nil {
			t.Fatal(err)
		}
		database.Exec(t, "UPDATE users SET role = $1 WHERE id = $2", string(role), user.ID)
		token, hash, err := opaquetoken.New("sos_")
		if err != nil {
			t.Fatal(err)
		}
		if err := database.Queries().CreateSession(ctx, dbsqlc.CreateSessionParams{TokenHash: hash, UserID: user.ID, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}}); err != nil {
			t.Fatal(err)
		}
		s.sessions[role] = token
		s.personTokens["Bearer person-"+string(role)] = user.ID.String()
		if role == auth.RoleAdmin {
			s.adminID = user.ID.String()
		}

		account, err := database.Queries().CreateServiceAccount(ctx, dbsqlc.CreateServiceAccountParams{Name: "key-" + string(role), Role: string(role)})
		if err != nil {
			t.Fatal(err)
		}
		key, keyHash, err := opaquetoken.New("sok_")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Queries().CreateServiceAccountKey(ctx, dbsqlc.CreateServiceAccountKeyParams{
			ServiceAccountID: account.ID, Name: "test", DisplayPrefix: key[:12], TokenHash: keyHash,
		}); err != nil {
			t.Fatal(err)
		}
		s.keys[role] = key
	}
}

func (s *stack) seedAuditContent(database *dbtest.Database, dashboardsService *dashboards.Service, access *folders.Access, runs *revisions.Service, logger *slog.Logger) {
	t := s.t
	ctx := datasource.WithAccess(t.Context(), datasource.Access{System: true})
	ids := trailtest.Insert(t, database, trailtest.Event{At: time.Now().Add(-time.Minute), Action: "dashboard.update", TargetKind: "dashboard", TargetName: marker, Actor: "Ada"})
	s.eventID = ids[0]

	if err := auditfolder.New(database.DB, dashboardsService, access, provision.Principal, logger).Seed(ctx); err != nil {
		t.Fatalf("seed the audit folder: %v", err)
	}
	if err := database.SQLDB(t).QueryRowContext(ctx, `SELECT id::text FROM folders WHERE title = 'Audit'`).Scan(&s.auditFolder); err != nil {
		t.Fatal(err)
	}
	rows, err := database.SQLDB(t).QueryContext(ctx, `SELECT id::text FROM dashboards WHERE folder_id = $1 ORDER BY id`, s.auditFolder)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		s.starters = append(s.starters, id)
	}
	if len(s.starters) == 0 {
		t.Fatal("the Audit folder holds no starter dashboard")
	}
	if err := database.SQLDB(t).QueryRowContext(ctx, `SELECT title FROM dashboards WHERE id = $1`, s.starters[0]).Scan(&s.starterTitle); err != nil {
		t.Fatal(err)
	}

	if err := database.SQLDB(t).QueryRowContext(ctx, `SELECT spec->'panels'->0->>'id' FROM dashboard_versions WHERE dashboard_id = $1 AND version = 1`, s.starters[0]).Scan(&s.starterPanel); err != nil {
		t.Fatal(err)
	}

	folder, _, err := access.CreateFolder(ctx, provision.Principal, controlFolderName)
	if err != nil {
		t.Fatal(err)
	}
	s.controlDir = folder.ID.String()
	spec := fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[{"id":"p","type":"text","title":"p","gridPos":{"x":0,"y":0,"w":12,"h":4},"content":"control"}]}`, controlBoard)
	dash, _, err := dashboardsService.Create(ctx, provision.Principal, folder.ID, []byte(spec), "control")
	if err != nil {
		t.Fatal(err)
	}
	s.controlBoard = dash.ID
	s.seedRuns(database, runs)
}

func (s *stack) seedRuns(database *dbtest.Database, runs *revisions.Service) {
	t := s.t
	ctx := datasource.WithAccess(t.Context(), datasource.Access{System: true})
	admin := auth.Principal{Kind: auth.KindUser, ID: s.adminID, Name: title(auth.RoleAdmin), Role: auth.RoleAdmin}
	dashboardID, err := db.ParseUUID(s.controlBoard)
	if err != nil {
		t.Fatal(err)
	}
	finish := func(id pgtype.UUID, restricted bool) string {
		database.Exec(t, "UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', read_restricted = $2 WHERE id = $1", id, restricted)
		return id.String()
	}
	enqueue := func(prompt string, restricted bool) string {
		row, err := runs.Enqueue(ctx, database.Queries(), revisions.EnqueueParams{DashboardID: dashboardID, Prompt: prompt, BaseVersion: 1, By: admin})
		if err != nil {
			t.Fatalf("enqueue a run on the control dashboard: %v", err)
		}
		return finish(row.ID, restricted)
	}
	s.restrictedRun = enqueue("who read the audit trail", true)
	s.plainRun = enqueue("what does the control board show", false)
	chat, err := runs.SubmitObserve(ctx, admin, "which people were denied")
	if err != nil {
		t.Fatalf("start a chat: %v", err)
	}
	s.restrictedChat = finish(chat.ID, true)
}

func (s *stack) previewBody() string {
	return fmt.Sprintf(`{"dashboard_id":%q,"version":1,"panel_id":"draft","panel":{"id":"draft","type":"stat","title":"draft","gridPos":{"x":0,"y":200,"w":6,"h":4},"datasource":%q,"queries":[{"query":"action:dashboard.update"}]}}`, s.starters[0], auditUID)
}

type response struct {
	status int
	body   string
	header http.Header
}

func (s *stack) authorize(req *http.Request, c caller) {
	switch c.kind {
	case personSession:
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: s.sessions[c.role], Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		req.Header.Set("Origin", s.server.URL)
	case serviceAccountKey:
		req.Header.Set("Authorization", "Bearer "+s.keys[c.role])
	}
}

func (s *stack) rest(c caller, method, path string, body string) response {
	s.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(s.t.Context(), method, s.server.URL+"/api/v1"+path, reader)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	s.authorize(req, c)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return response{status: res.StatusCode, body: string(raw), header: res.Header}
}

func (s *stack) stream(c caller, path string) response {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(s.t.Context(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.server.URL+"/api/v1"+path, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	s.authorize(req, c)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		return response{status: res.StatusCode, body: string(raw), header: res.Header}
	}
	var seen bytes.Buffer
	chunk := make([]byte, 512)
	for !strings.Contains(seen.String(), "event: ready") {
		n, err := res.Body.Read(chunk)
		seen.Write(chunk[:n])
		if err != nil {
			break
		}
	}
	return response{status: res.StatusCode, body: seen.String(), header: res.Header}
}

type toolAnswer struct {
	status  int
	isError bool
	text    string
	code    string
}

func (s *stack) mcpHeader(c caller) string {
	switch c.kind {
	case personSession:
		return "Bearer person-" + string(c.role)
	case serviceAccountKey:
		return "Bearer " + s.keys[c.role]
	}
	return ""
}

func (s *stack) call(c caller, name string, args map[string]any) toolAnswer {
	s.t.Helper()
	payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if err != nil {
		s.t.Fatal(err)
	}
	return s.mcpPost(c, payload)
}

func (s *stack) mcpPost(c caller, payload []byte) toolAnswer {
	s.t.Helper()
	req, err := http.NewRequestWithContext(s.t.Context(), http.MethodPost, s.server.URL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	if header := s.mcpHeader(c); header != "" {
		req.Header.Set("Authorization", header)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	out := toolAnswer{status: res.StatusCode, text: string(raw)}
	var envelope struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Structured struct {
				Code string `json:"code"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if res.StatusCode == http.StatusOK && json.Unmarshal(raw, &envelope) == nil {
		out.isError = envelope.Result.IsError
		out.code = envelope.Result.Structured.Code
		for _, part := range envelope.Result.Content {
			out.text += part.Text
		}
	}
	return out
}
