package cli

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/links"
)

const (
	clearLine = "\r\x1b[K"
	rowType   = "row"
)

type buildEnvelope struct {
	Build struct {
		DashboardID   *string   `json:"dashboard_id"`
		BaseVersion   *int      `json:"base_version"`
		PlannedPanels int       `json:"planned_panels"`
		Spec          shownSpec `json:"spec"`
	} `json:"build"`
}

type requestProgress struct {
	c   *client
	out *output
	id  string

	steps   stepProgress
	saves   int
	planned int
	seeded  bool
	seen    map[string]bool

	status string
	drawn  bool
}

func newRequestProgress(c *client, out *output, id string) *requestProgress {
	return &requestProgress{c: c, out: out, id: id, seen: map[string]bool{}}
}

func (p *requestProgress) live() bool {
	return p.out.terminal && !p.out.quiet && !p.out.json
}

func (p *requestProgress) update(ctx context.Context, req agentRequest) {
	if p.out.quiet {
		return
	}
	if p.live() {
		p.status = latestStep(req.Steps)
		p.drawStatus()
	} else {
		p.steps.print(p.out, req.Steps)
	}
	if req.Build == nil || req.Build.Saves <= p.saves {
		return
	}
	p.saves = req.Build.Saves
	p.planned = req.Build.PlannedPanels
	data, err := p.c.get(ctx, "observe/"+p.id+"/build", nil)
	if err != nil {
		return
	}
	build, err := decode[buildEnvelope](data)
	if err != nil {
		return
	}
	if !p.seeded {
		p.seeded = true
		if build.Build.BaseVersion != nil && build.Build.DashboardID != nil && !p.seedFromVersion(ctx, *build.Build.DashboardID, *build.Build.BaseVersion) {
			p.mark(build.Build.Spec.Panels)
			return
		}
	}
	p.report(build.Build.Spec.Panels)
}

func (p *requestProgress) seedFromVersion(ctx context.Context, dashboardID string, version int) bool {
	data, err := p.c.get(ctx, "dashboards/"+dashboardID+"/versions/"+strconv.Itoa(version), nil)
	if err != nil {
		return false
	}
	base, err := decode[dashboardWithVersion](data)
	if err != nil {
		return false
	}
	p.mark(base.Version.Spec.Panels)
	return true
}

func (p *requestProgress) mark(panels []shownPanel) {
	eachPanel(panels, func(panel shownPanel) { p.seen[panel.ID] = true })
}

func (p *requestProgress) report(panels []shownPanel) {
	eachPanel(panels, func(panel shownPanel) {
		if p.seen[panel.ID] {
			return
		}
		p.seen[panel.ID] = true
		p.say("%s", p.panelLine(len(p.seen), panel))
	})
}

func (p *requestProgress) panelLine(index int, panel shownPanel) string {
	label := "Panel " + strconv.Itoa(index)
	if p.planned > 0 && index <= p.planned {
		label += " of " + strconv.Itoa(p.planned)
	}
	title := panel.Title
	if title == "" {
		title = panel.ID
	}
	return fmt.Sprintf("%s %s (%s)", p.lineColor().dim(label+":"), text(title), text(panel.Type))
}

func (p *requestProgress) lineColor() palette {
	if p.live() {
		return p.out.color
	}
	return palette{}
}

func eachPanel(panels []shownPanel, visit func(shownPanel)) {
	for _, panel := range panels {
		if panel.Type != rowType {
			visit(panel)
		}
		eachPanel(panel.Panels, visit)
	}
}

func (p *requestProgress) say(format string, args ...any) {
	if !p.live() {
		p.out.notice(format, args...)
		return
	}
	p.clearStatus()
	p.out.line(format, args...)
	p.drawStatus()
}

func (p *requestProgress) drawStatus() {
	if p.status == "" {
		return
	}
	_, _ = fmt.Fprint(p.out.out, clearLine+p.out.color.dim(truncate(p.status, terminalWidth(p.out.out)-1)))
	p.drawn = true
}

func (p *requestProgress) stop() {
	p.clearStatus()
	p.status = ""
}

func (p *requestProgress) clearStatus() {
	if !p.drawn {
		return
	}
	_, _ = fmt.Fprint(p.out.out, clearLine)
	p.drawn = false
}

func latestStep(steps []agentStep) string {
	if len(steps) == 0 {
		return ""
	}
	last := steps[len(steps)-1]
	return text(last.Phase) + "  " + text(last.Message)
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if width < 1 || len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}

func (p *requestProgress) finish(ctx context.Context, dashboardID string) (string, bool) {
	data, err := p.c.get(ctx, "dashboards/"+dashboardID, nil)
	if err != nil {
		return "", false
	}
	saved, err := decode[dashboardWithVersion](data)
	if err != nil {
		return "", false
	}
	if p.seeded && !p.out.quiet {
		p.report(saved.Version.Spec.Panels)
	}
	return saved.Dashboard.Title, true
}

func titled(address, dashboardID, title string) string {
	base, ok := strings.CutSuffix(address, "/d/"+dashboardID)
	if !ok {
		return address
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return address
	}
	return links.Dashboard(parsed, dashboardID, title)
}

func reportSaved(out *output, version *int, address string) {
	if version == nil {
		out.line("%s", address)
		return
	}
	saved := "Saved version " + strconv.Itoa(*version)
	if out.terminal && !out.quiet {
		out.line("%s  %s", out.color.green(saved), address)
		return
	}
	out.notice("%s", saved)
	out.line("%s", address)
}
