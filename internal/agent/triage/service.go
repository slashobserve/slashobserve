package triage

import (
	"context"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/auditsource"
)

type Datasources interface {
	VisibleToEveryone(ctx context.Context) []datasource.Instance
}

type Service struct {
	Tools       Tools
	Datasources Datasources
	Limits      Limits
}

func (s Service) Sweep(ctx context.Context, run tools.Run) Outcome {
	ctx = datasource.WithAccess(tools.WithRun(ctx, run), run.Access)
	in := Input{Sources: SourcesOf(s.Datasources.VisibleToEveryone(ctx)), End: tools.Now(ctx)}
	if !run.To.IsZero() {
		in.End = run.To
	}
	if !run.From.IsZero() && !run.To.IsZero() {
		in.Question = &Window{From: run.From, To: run.To}
	}
	return Sweeper{Reader: ToolReader{Tools: s.Tools}, Limits: s.Limits}.Sweep(ctx, in)
}

func SourcesOf(instances []datasource.Instance) []Source {
	var out []Source
	for _, inst := range instances {
		if inst.UID == auditsource.UID || inst.Audience == datasource.AudienceAdmins {
			continue
		}
		t := inst.Datasource.Type()
		if len(datasource.ServiceSignals(t)) > 0 {
			out = append(out, Source{UID: inst.UID, Type: t})
		}
	}
	slices.SortFunc(out, func(a, b Source) int { return strings.Compare(a.UID, b.UID) })
	return out
}
