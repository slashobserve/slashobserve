package dashboards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	restoreLookback  = 10
	namedPanelsShown = 4
	titleShownRunes  = 60
)

type versionNote struct {
	summary      string
	restoredFrom *int
}

func (s *Service) describeBuild(ctx context.Context, build Build) (versionNote, error) {
	note := versionNote{summary: build.Summary}
	if !build.DashboardID.Valid {
		if summary := creationSummary(build.Spec); summary != "" {
			note.summary = summary
		}
		return note, nil
	}
	base, err := s.VersionSpec(ctx, build.DashboardID, build.BaseVersion)
	if err != nil {
		return versionNote{}, err
	}
	changes := changeParts(base, build.Spec)
	if len(changes) == 0 {
		return note, nil
	}
	note.summary = changeSentence(changes)
	reverted, err := s.revertedTo(ctx, build.DashboardID, build.BaseVersion, build.Spec)
	if err != nil {
		return versionNote{}, err
	}
	if reverted != nil {
		note.restoredFrom = reverted
		note.summary = fmt.Sprintf("Restored version %d (%s).", *reverted, strings.Join(changes, "; "))
	}
	return note, nil
}

func (s *Service) revertedTo(ctx context.Context, id pgtype.UUID, base int, final *spec.Dashboard) (*int, error) {
	want, err := json.Marshal(final)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	for version := base - 1; version >= 1 && version >= base-restoreLookback; version-- {
		earlier, err := s.VersionSpec(ctx, id, version)
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		have, err := json.Marshal(earlier)
		if err != nil {
			return nil, fmt.Errorf("marshal spec: %w", err)
		}
		if bytes.Equal(have, want) {
			return &version, nil
		}
	}
	return nil, nil
}

func creationSummary(d *spec.Dashboard) string {
	var titles []string
	count := 0
	for _, e := range d.AllPanels() {
		if e.Panel.Type == spec.Row {
			continue
		}
		count++
		titles = append(titles, panelName(e.Panel))
	}
	if count == 0 {
		return ""
	}
	return clipSummary(fmt.Sprintf("Built the dashboard with %s: %s.", countOf(count, "panel", "panels"), nameList(titles)))
}

func changeSentence(parts []string) string {
	joined := strings.Join(parts, "; ")
	_, size := utf8.DecodeRuneInString(joined)
	return clipSummary(strings.ToUpper(joined[:size]) + joined[size:] + ".")
}

func changeParts(before, after *spec.Dashboard) []string {
	var parts []string
	parts = append(parts, panelParts(before, after)...)
	parts = append(parts, settingParts(before, after)...)
	return parts
}

type placedPanel struct {
	panel *spec.Panel
	row   string
}

func placedPanels(d *spec.Dashboard) (map[string]placedPanel, []string) {
	byID := map[string]placedPanel{}
	var order []string
	for _, e := range d.AllPanels() {
		row := ""
		if e.Row != nil {
			row = e.Row.ID
		}
		byID[e.Panel.ID] = placedPanel{panel: e.Panel, row: row}
		order = append(order, e.Panel.ID)
	}
	return byID, order
}

func panelParts(before, after *spec.Dashboard) []string {
	was, wasOrder := placedPanels(before)
	now, nowOrder := placedPanels(after)
	var added, removed, changed, moved []string
	var renamed []string
	for _, id := range nowOrder {
		next := now[id]
		prev, existed := was[id]
		switch {
		case !existed:
			added = append(added, panelName(next.panel))
		case prev.panel.Title != next.panel.Title:
			renamed = append(renamed, fmt.Sprintf("%s to %s", quoted(panelName(prev.panel)), quoted(panelName(next.panel))))
			if !sameWithoutPlacement(prev.panel, next.panel, true) {
				changed = append(changed, panelName(next.panel))
			}
		case !sameWithoutPlacement(prev.panel, next.panel, false):
			changed = append(changed, panelName(next.panel))
		case prev.panel.GridPos != next.panel.GridPos || prev.row != next.row:
			moved = append(moved, panelName(next.panel))
		}
	}
	for _, id := range wasOrder {
		if _, kept := now[id]; !kept {
			removed = append(removed, panelName(was[id].panel))
		}
	}
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "added "+nameList(added))
	}
	if len(removed) > 0 {
		parts = append(parts, "removed "+nameList(removed))
	}
	if len(renamed) > 0 {
		parts = append(parts, "renamed "+strings.Join(renamed[:min(len(renamed), 2)], " and ")+moreSuffix(len(renamed), 2))
	}
	if len(changed) > 0 {
		parts = append(parts, "changed "+nameList(changed))
	}
	if len(moved) > 0 {
		parts = append(parts, "moved "+nameList(moved))
	}
	return parts
}

func sameWithoutPlacement(a, b *spec.Panel, ignoreTitle bool) bool {
	x, y := *a, *b
	x.Panels, y.Panels = nil, nil
	x.GridPos, y.GridPos = spec.GridPos{}, spec.GridPos{}
	if ignoreTitle {
		x.Title, y.Title = "", ""
	}
	left, errLeft := json.Marshal(x)
	right, errRight := json.Marshal(y)
	return errLeft == nil && errRight == nil && bytes.Equal(left, right)
}

func settingParts(before, after *spec.Dashboard) []string {
	var parts []string
	if before.Title != after.Title {
		parts = append(parts, "renamed the dashboard to "+quoted(clipRunes(after.Title, titleShownRunes)))
	}
	if before.Description != after.Description {
		parts = append(parts, "changed the description")
	}
	if !slices.Equal(before.Tags, after.Tags) {
		parts = append(parts, "changed the tags")
	}
	if before.Time != after.Time {
		parts = append(parts, "changed the time range")
	}
	if !sameJSON(before.Variables, after.Variables) {
		parts = append(parts, "changed the variables")
	}
	if !sameJSON(before.Links, after.Links) {
		parts = append(parts, "changed the links")
	}
	if len(parts) == 0 && !sameJSON(withoutPanels(before), withoutPanels(after)) {
		parts = append(parts, "changed the dashboard settings")
	}
	return parts
}

func withoutPanels(d *spec.Dashboard) *spec.Dashboard {
	head := *d
	head.Panels = nil
	return &head
}

func sameJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

func panelName(p *spec.Panel) string {
	if p.Title != "" {
		return clipRunes(p.Title, titleShownRunes)
	}
	return p.ID
}

func quoted(name string) string {
	return `"` + name + `"`
}

func nameList(names []string) string {
	shown := names[:min(len(names), namedPanelsShown)]
	quotedNames := make([]string, len(shown))
	for i, name := range shown {
		quotedNames[i] = quoted(name)
	}
	list := strings.Join(quotedNames[:max(len(quotedNames)-1, 1)], ", ")
	if len(quotedNames) > 1 {
		list += " and " + quotedNames[len(quotedNames)-1]
	}
	if rest := len(names) - len(shown); rest > 0 {
		list = strings.Join(quotedNames, ", ") + moreSuffix(len(names), namedPanelsShown)
	}
	return list
}

func moreSuffix(total, shown int) string {
	if total <= shown {
		return ""
	}
	return fmt.Sprintf(" and %d more", total-shown)
}

func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func clipRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit-3]) + "..."
}

func clipSummary(s string) string {
	return clipRunes(s, maxSummaryLength)
}
