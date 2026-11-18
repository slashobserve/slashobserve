package dashboards

import (
	"strconv"

	"github.com/slashobserve/slashobserve/internal/usertext"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxTitleRunes = 200

func cleanTitles(d *spec.Dashboard) []spec.Issue {
	var issues []spec.Issue
	clean := func(path string, title *string, optional bool) {
		if optional && *title == "" {
			return
		}
		cleaned, err := usertext.Clean("title", *title, usertext.Line(maxTitleRunes))
		if err != nil {
			code := spec.CodeSchema
			if reason, ok := usertext.ReasonOf(err); ok {
				code = spec.IssueCode(reason)
			}
			issues = append(issues, spec.Issue{Path: path, Code: code, Message: err.Error()})
			return
		}
		*title = cleaned
	}
	var panels func(prefix string, list []spec.Panel)
	panels = func(prefix string, list []spec.Panel) {
		for i := range list {
			path := prefix + "/" + strconv.Itoa(i)
			clean(path+"/title", &list[i].Title, true)
			panels(path+"/panels", list[i].Panels)
		}
	}
	clean("/title", &d.Title, false)
	panels("/panels", d.Panels)
	return issues
}
