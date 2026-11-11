package grafanaconn

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/sealed"
	"github.com/slashobserve/slashobserve/pkg/grafana"
)

const (
	syncLeaseTTL    = 60 * time.Second
	syncRenewEvery  = 10 * time.Second
	syncResumeEvery = 10 * time.Second

	maxNameRunes   = 100
	maxURLBytes    = 2048
	maxTokenBytes  = 4096
	maxFolderDepth = 16

	tokenKeyLabel = "slashobserve grafana connection token v1"
)

var (
	ErrNoSecretKey     = errors.New("SLASHOBSERVE_SECRET_KEY must be set to store or use a Grafana connection token")
	ErrUnreadableToken = errors.New("the stored token cannot be read with the current SLASHOBSERVE_SECRET_KEY; set the connection's token again")
	ErrTokenRequired   = errors.New("a new url needs the token again")
	ErrTooManyDomains  = errors.New("the outbound allowlist is full")
	ErrUnknownUID      = errors.New("no imported dashboard has this grafana uid")
)

type InvalidError struct {
	Message string
}

func (e *InvalidError) Error() string { return e.Message }

type Principals interface {
	Principal(ctx context.Context, kind auth.PrincipalKind, id string) (auth.Principal, error)
}

type Deps struct {
	DB         *db.DB
	Egress     *egress.Client
	Domains    *allowlist.Store
	Imports    *imports.Service
	Access     *folders.Access
	Principals Principals
	InstanceID string
	SecretKey  []byte
	Logger     *slog.Logger
}

type Service struct {
	store      store
	egress     *egress.Client
	domains    *allowlist.Store
	imports    *imports.Service
	access     *folders.Access
	principals Principals
	instanceID string
	key        sealed.Key
	logger     *slog.Logger

	life        context.Context
	wg          sync.WaitGroup
	leaseTTL    time.Duration
	renewEvery  time.Duration
	resumeEvery time.Duration
}

func New(life context.Context, d Deps) *Service {
	return &Service{
		store: store{db: d.DB}, egress: d.Egress, domains: d.Domains, imports: d.Imports, access: d.Access,
		principals: d.Principals, instanceID: d.InstanceID, key: d.SecretKey, logger: d.Logger, life: life,
		leaseTTL: syncLeaseTTL, renewEvery: syncRenewEvery, resumeEvery: syncResumeEvery,
	}
}

func (s *Service) Wait() {
	s.wg.Wait()
}

type Input struct {
	Name  string
	URL   string
	Token string
}

type Patch struct {
	Name  *string
	URL   *string
	Token *string
}

type Folder struct {
	UID        string
	Title      string
	Dashboards int
}

type DatasourceMatch struct {
	UID        string
	Name       string
	Type       string
	Kind       string
	Candidates []string
	MatchedUID string
}

func (s *Service) sealToken(id pgtype.UUID, token string) ([]byte, error) {
	ciphertext, err := s.key.Seal(tokenKeyLabel, id, []byte(token))
	if errors.Is(err, sealed.ErrNoKey) {
		return nil, ErrNoSecretKey
	}
	return ciphertext, err
}

func (s *Service) openToken(id pgtype.UUID, ciphertext []byte) (string, error) {
	plain, err := s.key.Open(tokenKeyLabel, id, ciphertext)
	switch {
	case errors.Is(err, sealed.ErrNoKey):
		return "", ErrNoSecretKey
	case errors.Is(err, sealed.ErrUnreadable):
		return "", ErrUnreadableToken
	}
	return string(plain), err
}

func newID() pgtype.UUID {
	var id pgtype.UUID
	_, _ = rand.Read(id.Bytes[:])
	id.Bytes[6] = id.Bytes[6]&0x0f | 0x40
	id.Bytes[8] = id.Bytes[8]&0x3f | 0x80
	id.Valid = true
	return id
}

func cleanName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes || strings.ContainsFunc(name, unicode.IsControl) {
		return "", &InvalidError{Message: fmt.Sprintf("name must be 1 to %d characters without control characters", maxNameRunes)}
	}
	return name, nil
}

func cleanURL(raw string) (*url.URL, string, error) {
	invalid := &InvalidError{Message: "url must be an http or https address with a domain name and no user info, query or fragment"}
	text := strings.TrimSpace(raw)
	if text == "" || len(text) > maxURLBytes {
		return nil, "", invalid
	}
	u, err := url.Parse(text)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return nil, "", invalid
	}
	host, err := domains.NormalizeHost(u.Hostname())
	if err != nil {
		return nil, "", invalid
	}
	port := u.Port()
	if !validPort(port) {
		return nil, "", invalid
	}
	u.Host = host
	if port != "" {
		u.Host = host + ":" + port
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u, host, nil
}

func validPort(port string) bool {
	if port == "" {
		return true
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func cleanToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" || len(token) > maxTokenBytes || strings.ContainsFunc(token, func(r rune) bool { return r < '!' || r > '~' }) {
		return "", &InvalidError{Message: fmt.Sprintf("token must be 1 to %d visible ASCII characters", maxTokenBytes)}
	}
	return token, nil
}

func (s *Service) ensureAllowed(ctx context.Context, by auth.Principal, host string) error {
	list, err := s.domains.Allowed(ctx)
	if err != nil {
		return err
	}
	if list.Allows(host) {
		return nil
	}
	pattern, err := domains.ParsePattern(host)
	if err != nil {
		return &InvalidError{Message: "url must name a domain"}
	}
	_, err = s.domains.Add(ctx, by, pattern)
	switch {
	case err == nil:
		s.logger.InfoContext(ctx, "allowed an outbound domain", slog.String("actor_id", by.ID), slog.String("domain", pattern.String()))
		return nil
	case errors.Is(err, allowlist.ErrExists):
		return nil
	case errors.Is(err, allowlist.ErrLocked):
		return &egress.HostError{Host: host, Err: egress.ErrNotAllowed}
	case errors.Is(err, allowlist.ErrTooMany):
		return ErrTooManyDomains
	}
	return err
}

func (s *Service) Create(ctx context.Context, by auth.Principal, in Input) (Connection, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return Connection{}, err
	}
	u, host, err := cleanURL(in.URL)
	if err != nil {
		return Connection{}, err
	}
	token, err := cleanToken(in.Token)
	if err != nil {
		return Connection{}, err
	}
	if !s.key.Set() {
		return Connection{}, ErrNoSecretKey
	}
	if err := s.ensureAllowed(ctx, by, host); err != nil {
		return Connection{}, err
	}
	id := newID()
	ciphertext, err := s.sealToken(id, token)
	if err != nil {
		return Connection{}, err
	}
	params := dbsqlc.InsertGrafanaConnectionParams{ID: id, Name: name, Url: u.String(), TokenCiphertext: ciphertext}
	if creator, err := db.ParseUUID(by.ID); err == nil {
		params.CreatedBy = creator
	}
	created, err := s.store.insert(ctx, params)
	if err != nil {
		return Connection{}, err
	}
	s.logger.InfoContext(ctx, "grafana connection created", slog.String("connection_id", created.ID), slog.String("actor_id", by.ID), slog.String("host", host))
	return created.Connection, nil
}

func (s *Service) Update(ctx context.Context, by auth.Principal, id pgtype.UUID, p Patch) (Connection, error) {
	current, err := s.store.get(ctx, id)
	if err != nil {
		return Connection{}, err
	}
	params := dbsqlc.UpdateGrafanaConnectionParams{ID: id, Name: current.Name, Url: current.URL, TokenCiphertext: current.sealed}
	if p.Name != nil {
		if params.Name, err = cleanName(*p.Name); err != nil {
			return Connection{}, err
		}
	}
	host := ""
	if p.URL != nil {
		u, h, err := cleanURL(*p.URL)
		if err != nil {
			return Connection{}, err
		}
		params.ResetTest = u.String() != current.URL
		params.Url, host = u.String(), h
	}
	if params.ResetTest && p.Token == nil {
		return Connection{}, ErrTokenRequired
	}
	if p.Token != nil {
		token, err := cleanToken(*p.Token)
		if err != nil {
			return Connection{}, err
		}
		if params.TokenCiphertext, err = s.sealToken(id, token); err != nil {
			return Connection{}, err
		}
		params.ResetTest = true
	}
	if host != "" {
		if err := s.ensureAllowed(ctx, by, host); err != nil {
			return Connection{}, err
		}
	}
	updated, err := s.store.update(ctx, params)
	if err != nil {
		return Connection{}, err
	}
	s.logger.InfoContext(ctx, "grafana connection updated", slog.String("connection_id", updated.ID), slog.String("actor_id", by.ID), slog.Bool("token_changed", p.Token != nil))
	return updated.Connection, nil
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, id pgtype.UUID) error {
	if err := s.store.delete(ctx, id); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "grafana connection deleted", slog.String("connection_id", id.String()), slog.String("actor_id", by.ID))
	return nil
}

func (s *Service) List(ctx context.Context) ([]Connection, error) {
	return s.store.list(ctx)
}

func (s *Service) Get(ctx context.Context, id pgtype.UUID) (Connection, error) {
	c, err := s.store.get(ctx, id)
	return c.Connection, err
}

func (s *Service) client(ctx context.Context, id pgtype.UUID) (stored, grafanaClient, error) {
	c, err := s.store.get(ctx, id)
	if err != nil {
		return stored{}, grafanaClient{}, err
	}
	token, err := s.openToken(id, c.sealed)
	if err != nil {
		return stored{}, grafanaClient{}, err
	}
	base, err := url.Parse(c.URL)
	if err != nil {
		return stored{}, grafanaClient{}, fmt.Errorf("stored grafana url: %w", err)
	}
	return c, grafanaClient{egress: s.egress, base: base, token: token}, nil
}

func (s *Service) Test(ctx context.Context, id pgtype.UUID) (Connection, error) {
	_, client, err := s.client(ctx, id)
	if err != nil {
		return Connection{}, err
	}
	ok, message := true, ""
	if err := client.health(ctx); err != nil {
		ok, message = false, describe(err)
	}
	var folderCount int
	if ok {
		roots, err := client.rootFolders(ctx)
		if err != nil {
			ok, message = false, describe(err)
		}
		folderCount = len(roots)
	}
	if ok {
		hits, err := client.searchPage(ctx, "dash-db", 1)
		if err != nil {
			ok, message = false, describe(err)
		} else {
			message = connectedMessage(folderCount, len(hits))
		}
	}
	if ctx.Err() != nil {
		return Connection{}, ctx.Err()
	}
	tested, err := s.store.setTest(ctx, id, ok, message)
	if err != nil {
		return Connection{}, err
	}
	s.logger.InfoContext(ctx, "grafana connection tested", slog.String("connection_id", tested.ID), slog.Bool("ok", ok))
	return tested.Connection, nil
}

func connectedMessage(folderCount, dashboards int) string {
	count := fmt.Sprintf("%d dashboards", dashboards)
	if dashboards >= searchPageSize {
		count = fmt.Sprintf("%d or more dashboards", searchPageSize)
	}
	return fmt.Sprintf("Connected: %d top-level folders, %s.", folderCount, count)
}

type folderTree struct {
	byUID map[string]apiHit
}

func (t folderTree) title(uid string) string {
	if uid == "" {
		return rootFolderTitle
	}
	var parts []string
	for cur, depth := uid, 0; cur != "" && depth < maxFolderDepth; depth++ {
		hit, ok := t.byUID[cur]
		if !ok {
			break
		}
		parts = append(parts, strings.TrimSpace(hit.Title))
		cur = hit.FolderUID
	}
	slices.Reverse(parts)
	return strings.Join(parts, " / ")
}

func (t folderTree) within(uid, ancestor string) bool {
	for cur, depth := uid, 0; depth < maxFolderDepth; depth++ {
		if cur == ancestor {
			return true
		}
		hit, ok := t.byUID[cur]
		if cur == "" || !ok {
			return false
		}
		cur = hit.FolderUID
	}
	return false
}

func (s *Service) tree(ctx context.Context, client grafanaClient) (folderTree, error) {
	hits, err := client.searchAll(ctx, "dash-folder")
	if err != nil {
		return folderTree{}, err
	}
	t := folderTree{byUID: map[string]apiHit{}}
	for _, h := range hits {
		t.byUID[h.UID] = h
	}
	return t, nil
}

func (s *Service) Folders(ctx context.Context, id pgtype.UUID) ([]Folder, error) {
	_, client, err := s.client(ctx, id)
	if err != nil {
		return nil, err
	}
	tree, err := s.tree(ctx, client)
	if err != nil {
		return nil, err
	}
	dashboards, err := client.searchAll(ctx, "dash-db")
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, d := range dashboards {
		counts[d.FolderUID]++
	}
	out := make([]Folder, 0, len(tree.byUID)+1)
	for uid := range tree.byUID {
		out = append(out, Folder{UID: uid, Title: tree.title(uid), Dashboards: counts[uid]})
	}
	if counts[""] > 0 {
		out = append(out, Folder{UID: rootFolderUID, Title: rootFolderTitle, Dashboards: counts[""]})
	}
	slices.SortFunc(out, func(a, b Folder) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)), strings.Compare(a.UID, b.UID))
	})
	return out, nil
}

func (s *Service) Datasources(ctx context.Context, id pgtype.UUID) ([]DatasourceMatch, error) {
	_, client, err := s.client(ctx, id)
	if err != nil {
		return nil, err
	}
	list, err := client.datasources(ctx)
	if err != nil {
		return nil, err
	}
	ours := s.imports.Datasources()
	out := make([]DatasourceMatch, 0, len(list))
	for _, d := range list {
		choice := grafana.ChooseDatasource(d.UID, d.Name, d.Type, ours)
		out = append(out, DatasourceMatch{UID: d.UID, Name: d.Name, Type: d.Type, Kind: choice.Kind, Candidates: choice.Candidates, MatchedUID: choice.Match})
	}
	slices.SortFunc(out, func(a, b DatasourceMatch) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.UID, b.UID))
	})
	return out, nil
}

func (s *Service) Resolve(ctx context.Context, by auth.Principal, uid string) (pgtype.UUID, error) {
	found, ok, err := s.imports.DashboardByGrafanaUID(ctx, uid)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if !ok {
		return pgtype.UUID{}, ErrUnknownUID
	}
	_, err = s.access.Dashboard(ctx, by, found.DashboardID)
	if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) || errors.Is(err, folders.ErrForbidden) {
		return pgtype.UUID{}, ErrUnknownUID
	}
	if err != nil {
		return pgtype.UUID{}, err
	}
	return found.DashboardID, nil
}
