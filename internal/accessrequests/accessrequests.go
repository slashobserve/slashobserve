package accessrequests

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
)

type Kind string

const (
	KindDashboard   Kind = "dashboard"
	KindObservation Kind = "observation"
)

var Kinds = []Kind{KindDashboard, KindObservation}

func ParseKind(s string) (Kind, bool) {
	for _, k := range Kinds {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}

const (
	MaxNoteRunes = 200
	maxListed    = 100
)

var (
	ErrNotFound    = errors.New("not found")
	ErrCanComment  = errors.New("the caller can already comment here")
	ErrInvalidNote = fmt.Errorf("note must be one line of at most %d characters", MaxNoteRunes)
)

type OpenError struct {
	RequestedAt time.Time
}

func (e *OpenError) Error() string { return "the caller already has an open request here" }

func (e *OpenError) Message() string {
	return "you already asked for comment access here at " + e.RequestedAt.UTC().Format(time.RFC3339) + " and the request is still open"
}

type Object struct {
	Kind Kind
	ID   pgtype.UUID
}

type Requester struct {
	Kind string
	ID   string
	Name string
}

type Folder struct {
	ID      string
	Title   string
	General bool
}

type Request struct {
	ID          string
	Object      Object
	Title       string
	Folder      Folder
	RequestedBy Requester
	Note        string
	CreatedAt   time.Time
}

func folderOf(levels folders.Levels, id pgtype.UUID) Folder {
	for _, f := range levels.Folders() {
		if f.ID == id {
			return Folder{ID: id.String(), Title: f.Title, General: f.General}
		}
	}
	return Folder{ID: id.String()}
}

func cleanNote(raw string) (string, error) {
	note := strings.TrimSpace(raw)
	if strings.ContainsAny(note, "\r\n") || utf8.RuneCountInString(note) > MaxNoteRunes || !utf8.ValidString(note) {
		return "", ErrInvalidNote
	}
	return note, nil
}

func mayAsk(a folders.Abilities) error {
	switch {
	case !a.View:
		return ErrNotFound
	case a.Comment:
		return ErrCanComment
	}
	return nil
}

type candidate struct {
	id    pgtype.UUID
	role  auth.Role
	facts folders.Facts
}

func chooseGranters(candidates []candidate, admins []pgtype.UUID) []pgtype.UUID {
	out := []pgtype.UUID{}
	for _, c := range candidates {
		if folders.LevelFor(c.role, c.facts) == folders.LevelAdmin {
			out = append(out, c.id)
		}
	}
	if len(out) == 0 {
		return append(out, admins...)
	}
	return out
}

func levelsOf(raw []string) []folders.Level {
	out := make([]folders.Level, 0, len(raw))
	for _, g := range raw {
		if level, ok := folders.ParseLevel(g); ok {
			out = append(out, level)
		}
	}
	return out
}

func roleOf(raw string) auth.Role {
	role, ok := auth.ParseRole(raw)
	if !ok {
		return ""
	}
	return role
}

type Service struct {
	db     *db.DB
	access *folders.Access
}

func New(database *db.DB, access *folders.Access) *Service {
	return &Service{db: database, access: access}
}

func requesterOf(by auth.Principal) (Requester, pgtype.UUID, error) {
	if by.Kind != auth.KindUser && by.Kind != auth.KindServiceAccount {
		return Requester{}, pgtype.UUID{}, folders.ErrForbidden
	}
	id, err := db.ParseUUID(by.ID)
	if err != nil {
		return Requester{}, pgtype.UUID{}, folders.ErrForbidden
	}
	name := by.Name
	if name == "" {
		name = by.Email
	}
	return Requester{Kind: string(by.Kind), ID: by.ID, Name: name}, id, nil
}

type target struct {
	folder pgtype.UUID
	title  string
}

func lockTarget(ctx context.Context, q *dbsqlc.Queries, obj Object) (target, error) {
	var t target
	var err error
	switch obj.Kind {
	case KindDashboard:
		var row dbsqlc.LockAccessRequestDashboardRow
		row, err = q.LockAccessRequestDashboard(ctx, obj.ID)
		t = target{folder: row.FolderID, title: row.Title}
	case KindObservation:
		var row dbsqlc.LockAccessRequestObservationRow
		row, err = q.LockAccessRequestObservation(ctx, obj.ID)
		t = target{folder: row.FolderID, title: row.Title}
	default:
		return target{}, ErrNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return target{}, ErrNotFound
	}
	return t, err
}

func ownerOf(obj Object) activity.Owner {
	if obj.Kind == KindObservation {
		return activity.OnObservation(obj.ID)
	}
	return activity.OnDashboard(obj.ID)
}

type objectData struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
}

type openedData struct {
	Object  objectData `json:"object"`
	Title   string     `json:"title"`
	General bool       `json:"general,omitempty"`
}

type requesterData struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type grantedData struct {
	Object      objectData    `json:"object"`
	Title       string        `json:"title"`
	RequestedBy requesterData `json:"requested_by"`
}

func (s *Service) granters(ctx context.Context, q *dbsqlc.Queries, folder pgtype.UUID) ([]pgtype.UUID, error) {
	rows, err := q.AccessGranterCandidates(ctx, folder)
	if err != nil {
		return nil, fmt.Errorf("list access granters: %w", err)
	}
	candidates := make([]candidate, 0, len(rows))
	for _, r := range rows {
		candidates = append(candidates, candidate{id: r.ID, role: roleOf(r.Role), facts: folders.Facts{Restricted: folders.Restricted(r.AdminsOnly, r.Listed), AdminsOnly: r.AdminsOnly, Grants: levelsOf(r.Grants), Creator: r.Creator}})
	}
	admins, err := q.ActiveAdminIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admins: %w", err)
	}
	return chooseGranters(candidates, admins), nil
}

func (s *Service) Request(ctx context.Context, by auth.Principal, obj Object, rawNote string) (Request, error) {
	note, err := cleanNote(rawNote)
	if err != nil {
		return Request{}, err
	}
	requester, requesterID, err := requesterOf(by)
	if err != nil {
		return Request{}, err
	}
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return Request{}, err
	}
	out := Request{Object: obj, RequestedBy: requester, Note: note}
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		t, err := lockTarget(ctx, q, obj)
		if err != nil {
			return err
		}
		if err := mayAsk(levels.Of(t.folder)); err != nil {
			return err
		}
		out.Folder = folderOf(levels, t.folder)
		slot := dbsqlc.LockAccessRequestSlotParams{ObjectID: obj.ID, RequesterKind: requester.Kind, RequesterID: requesterID}
		if err := q.LockAccessRequestSlot(ctx, slot); err != nil {
			return fmt.Errorf("lock access request: %w", err)
		}
		open, err := q.OpenAccessRequestFor(ctx, dbsqlc.OpenAccessRequestForParams(slot))
		if err == nil {
			return &OpenError{RequestedAt: open.CreatedAt.Time}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("find open access request: %w", err)
		}
		granters, err := s.granters(ctx, q, t.folder)
		if err != nil {
			return err
		}
		id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
		seq, err := activity.Append(ctx, q, activity.Entry{
			Type:     activity.TypeAccessRequestOpened,
			TargetID: id,
			Owner:    ownerOf(obj),
			Actor:    activity.By(ctx, by),
			Data:     openedData{Object: objectData{Kind: obj.Kind, ID: obj.ID.String()}, Title: t.title, General: out.Folder.General},
		}, nil)
		if err != nil {
			return err
		}
		params := dbsqlc.InsertAccessRequestParams{
			ID: id, RequesterKind: requester.Kind, RequesterID: requesterID, RequesterName: requester.Name,
			Note: note, Granters: granters, ActivitySeq: seq,
		}
		if obj.Kind == KindObservation {
			params.ObservationID = obj.ID
		} else {
			params.DashboardID = obj.ID
		}
		created, err := q.InsertAccessRequest(ctx, params)
		if err != nil {
			return fmt.Errorf("insert access request: %w", err)
		}
		out.ID, out.Title, out.CreatedAt = id.String(), t.title, created.Time
		return nil
	})
	if err != nil {
		return Request{}, err
	}
	return out, nil
}

func (s *Service) List(ctx context.Context, by auth.Principal) ([]Request, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return nil, err
	}
	managed := []pgtype.UUID{}
	for _, f := range levels.Folders() {
		if levels.Of(f.ID).Manage {
			managed = append(managed, f.ID)
		}
	}
	rows, err := s.db.Queries().ListOpenAccessRequests(ctx, dbsqlc.ListOpenAccessRequestsParams{Folders: managed, RowLimit: maxListed})
	if err != nil {
		return nil, fmt.Errorf("list access requests: %w", err)
	}
	out := make([]Request, 0, len(rows))
	for _, r := range rows {
		obj := Object{Kind: KindDashboard, ID: r.DashboardID}
		if r.ObservationID.Valid {
			obj = Object{Kind: KindObservation, ID: r.ObservationID}
		}
		out = append(out, Request{
			ID:          r.ID.String(),
			Object:      obj,
			Title:       r.Title,
			Folder:      folderOf(levels, r.FolderID),
			RequestedBy: Requester{Kind: r.RequesterKind, ID: r.RequesterID.String(), Name: r.RequesterName},
			Note:        r.Note,
			CreatedAt:   r.CreatedAt.Time,
		})
	}
	return out, nil
}
