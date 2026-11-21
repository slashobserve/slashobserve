package tools

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
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
	case a.Audience != datasource.AudienceCaller || !a.Reads(datasource.AudienceAdmins):
		return listEveryTitle
	case include:
		return listRestrictedTitles
	}
	return leaveOutRestrictedTitles
}

type restrictedTitles struct {
	rule       titleRule
	adminsOnly map[pgtype.UUID]bool
	builtIn    map[string]bool
	leftOut    int
	listed     bool
}

func (r *Registry) restrictedTitles(ctx context.Context, include bool) (*restrictedTitles, Result, bool, error) {
	run, inRun := RunFrom(ctx)
	t := &restrictedTitles{rule: titleRuleFor(run, inRun, include)}
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
	if t.rule == listEveryTitle || !t.adminsOnly[folder] || t.builtIn[id] {
		return true
	}
	if t.rule == leaveOutRestrictedTitles {
		t.leftOut++
		return false
	}
	t.listed = true
	return true
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
