package grafana

import (
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxDashListQuery = 200
	maxDashListTags  = 10
	maxDashListItems = 100
	dashListLimit    = "Only 1 to 100 dashboards can be listed."
	dashListTagLimit = "A dashboard list holds at most 10 tags."
	dashListTagBad   = "This tag cannot be matched by a SlashObserve tag."
	dashListSources  = "Starred and recently viewed lists are not available; the panel lists dashboards by last update."
	dashListFolders  = "Folders are not supported."
	dashListRepeat   = "Repeating dashboard lists is not supported yet."
)

func (b *panelBuild) buildDashList() *builtPanel {
	b.panel.Title = b.title()
	b.panel.Description = b.description()
	d := b.disp()
	d.Query = b.dashListQuery()
	d.Tags = b.dashListTags()
	b.dashListLimit(d)
	d.KeepTime, _ = b.boolOption("options.keepTime")
	d.IncludeVars, _ = b.boolOption("options.includeVars")
	b.dashListSources()
	if asString(b.raw["repeat"]) != "" {
		b.partial("repeat", dashListRepeat)
	}
	b.convertLinks()
	b.reportUnconsumed()
	return b.finish()
}

func (b *panelBuild) dashListQuery() string {
	v, _ := b.take("options.query")
	query := strings.TrimSpace(asString(v))
	cut, wasCut := cutRunes(query, maxDashListQuery)
	if wasCut {
		b.partial("options.query", "The search text was cut to 200 characters.")
	}
	return cut
}

func (b *panelBuild) dashListTags() []string {
	v, _ := b.take("options.tags")
	items, _ := asArray(v)
	var tags []string
	for i, item := range items {
		path := fmt.Sprintf("options.tags[%d]", i)
		tag := tagInvalid.ReplaceAllString(strings.ToLower(asString(item)), "-")
		tag, _ = cutRunes(tag, 40)
		switch {
		case !tagPattern.MatchString(tag):
			b.partial(path, dashListTagBad)
		case slices.Contains(tags, tag):
		case len(tags) >= maxDashListTags:
			b.partial(path, dashListTagLimit)
		default:
			tags = append(tags, tag)
		}
	}
	return tags
}

func (b *panelBuild) dashListLimit(d *spec.Display) {
	v, ok := b.take("options.maxItems")
	if !ok {
		return
	}
	if n, isInt := asInt(v); isInt && n >= 1 && n <= maxDashListItems {
		d.Limit = n
		return
	}
	b.partial("options.maxItems", dashListLimit)
}

func (b *panelBuild) dashListSources() {
	for _, key := range []string{"showStarred", "showRecentlyViewed"} {
		path := "options." + key
		if truthy(lookupValue(b.raw, path)) {
			b.partial(path, dashListSources)
		}
		b.consume(path)
	}
	b.consume("options.showSearch")
	for _, path := range []string{"options.folderUID", "options.folderId"} {
		if v := lookupValue(b.raw, path); present(v) && !sameJSON(v, 0) {
			b.partial(path, dashListFolders)
		}
		b.consume(path)
	}
}
