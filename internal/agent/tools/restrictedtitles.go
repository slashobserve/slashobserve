package tools

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
)

type titleRule int

const (
	listEveryTitle titleRule = iota
	leaveOutRestrictedTitles
	listRestrictedTitles
	refuseRestrictedTitles
)

func titleRuleFor(run Run, inRun, include bool) titleRule {
	a := run.Access
	switch {
	case !inRun:
		return listEveryTitle
	case a.Audience == datasource.AudienceEveryone && include:
		return refuseRestrictedTitles
	case !a.Reads(datasource.AudienceAdmins):
		return listEveryTitle
	case include:
		return listRestrictedTitles
	}
	return leaveOutRestrictedTitles
}

type restrictedTitles struct {
	rule       titleRule
	run        Run
	adminsOnly map[pgtype.UUID]bool
	builtIn    map[string]bool
	leftOut    int
	listed     bool
}

func (r *Registry) restrictedTitles(ctx context.Context, include bool) (*restrictedTitles, Result, bool, error) {
	run, inRun := RunFrom(ctx)
	t := &restrictedTitles{rule: titleRuleFor(run, inRun, include), run: run}
	switch t.rule {
	case refuseRestrictedTitles:
		return nil, invalid("include_restricted is refused here. %s", datasource.WithheldNote), false, nil
	case listEveryTitle:
		return t, Result{}, true, nil
	}
	if r.deps.Folders == nil {
		return nil, Result{}, false, errFoldersNotConfigured
	}
	principal, _ := auth.FromContext(ctx)
	levels, err := r.deps.Folders.Levels(ctx, principal)
	if err != nil {
		return nil, Result{}, false, err
	}
	t.adminsOnly = map[pgtype.UUID]bool{}
	for _, f := range levels.Folders() {
		t.adminsOnly[f.ID] = f.AdminsOnly
	}
	builtIn, err := r.deps.Folders.BuiltIn(ctx)
	if err != nil {
		return nil, Result{}, false, err
	}
	t.builtIn = map[string]bool{}
	for id := range builtIn {
		t.builtIn[id.String()] = true
	}
	return t, Result{}, true, nil
}

func (t *restrictedTitles) keep(folder pgtype.UUID, id string) bool {
	if t.rule == listEveryTitle || !t.adminsOnly[folder] || t.builtIn[id] || t.run.about(id) || slices.Contains(t.run.WriteFolders, folder) {
		return true
	}
	if t.rule == leaveOutRestrictedTitles {
		t.leftOut++
		return false
	}
	t.listed = true
	return true
}

func (t *restrictedTitles) keepIn(folderID, id string) bool {
	folder, _ := db.ParseUUID(folderID)
	return t.keep(folder, id)
}

func (t *restrictedTitles) keepAdminsOnly() bool {
	if t.rule != leaveOutRestrictedTitles {
		return true
	}
	t.leftOut++
	return false
}

func (t *restrictedTitles) holds(ctx context.Context, folderID, id string) {
	if !t.keepIn(folderID, id) {
		datasource.NoteRead(ctx, datasource.AudienceAdmins)
	}
}

func (t *restrictedTitles) restart() {
	t.leftOut, t.listed = 0, false
}

func (t *restrictedTitles) done(ctx context.Context) {
	if t.listed {
		datasource.NoteRead(ctx, datasource.AudienceAdmins)
	}
}

func (t *restrictedTitles) note(one, many string) string {
	if t.leftOut == 0 {
		return ""
	}
	return "Left out " + plural(t.leftOut, one, many) + " in folders only Admins can read; call again with include_restricted: true to list them, and your answer then stays where only Admins read it."
}

type leftOutOut struct {
	LeftOut    int    `json:"left_out,omitempty"`
	LeftOutWhy string `json:"left_out_note,omitempty"`
}

func (t *restrictedTitles) report(one, many string) leftOutOut {
	if t.leftOut == 0 {
		return leftOutOut{}
	}
	return leftOutOut{LeftOut: t.leftOut, LeftOutWhy: "Left out " + plural(t.leftOut, one, many) + " in folders only Admins can read; a private chat lists them."}
}

func keepReadable[T any](t *restrictedTitles, items []T, at func(T) (folderID, id string)) []T {
	t.restart()
	return slices.DeleteFunc(items, func(item T) bool {
		folderID, id := at(item)
		return !t.keepIn(folderID, id)
	})
}
