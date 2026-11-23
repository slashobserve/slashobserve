package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	nameListServices   = "list_services"
	servicesConcurrent = 4
	maxServiceBytes    = 200
	descListServices   = "List the services a log or trace datasource has seen over a recent window, or find which datasources hold a service. Pass datasource for one source; leave it out to ask every Loki, Tempo, Jaeger, ClickHouse, Elastic or OpenSearch source. Pass service to keep only the sources that hold a name containing it, case-insensitive. The window ends now and defaults to 1 hour, at most 6 hours, so a service quiet for longer is not listed, except on Jaeger, which lists every service it retains whatever the window. Each source says which field holds the service, so filter on that field in a query. A source that cannot answer in time says so under unavailable and the others still answer. Answers are cached for about a minute."
	propServicesSignal = `"signal":{"type":"string","enum":["logs","traces"],"description":"Which signal. Default: logs, and traces on Tempo and Jaeger; ClickHouse holds both and answers both when this is left out and no datasource is given."}`
	propServicesWindow = `"window":{"type":"string","maxLength":16,"description":"How far back to look, such as 15m or 1h. Default 1h, at most 6h."}`
	propServicesFind   = `"service":{"type":"string","maxLength":200,"description":"Keep only sources with a service whose name contains this text."}`
	schemaListServices = `{"type":"object","properties":{"datasource":{"type":"string","description":"Datasource uid from list_datasources. Leave out to ask every log and trace source."},` + propServicesSignal + `,` + propServicesWindow + `,` + propServicesFind + `},"additionalProperties":false}`
)

func init() { registerEntries((*Registry).serviceEntries) }

func (r *Registry) serviceEntries() []entry {
	return []entry{
		{Tool{Name: nameListServices, Description: descListServices, InputSchema: json.RawMessage(schemaListServices), ReadOnly: true, MinRole: auth.RoleViewer}, r.listServices},
	}
}

type serviceSourceOut struct {
	Datasource  string   `json:"datasource"`
	Type        string   `json:"type"`
	Signal      string   `json:"signal"`
	Field       string   `json:"field,omitempty"`
	Services    []string `json:"services"`
	Truncated   bool     `json:"truncated,omitempty"`
	Unavailable string   `json:"unavailable,omitempty"`
}

type servicesOut struct {
	WindowSeconds int                `json:"window_seconds"`
	Sources       []serviceSourceOut `json:"sources"`
	Truncated     bool               `json:"truncated,omitempty"`
}

func (r *Registry) listServices(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource string `json:"datasource"`
		Signal     string `json:"signal"`
		Window     string `json:"window"`
		Service    string `json:"service"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if res, ok := checkLength("service", in.Service, maxServiceBytes); !ok {
		return res, nil
	}
	if in.Signal != "" && in.Signal != datasource.SignalLogs && in.Signal != datasource.SignalTraces {
		return invalid("signal must be logs or traces"), nil
	}
	var window time.Duration
	if in.Window != "" {
		parsed, err := time.ParseDuration(in.Window)
		if err != nil {
			return invalid("window must be a duration such as 15m or 1h"), nil
		}
		window = parsed
	}
	targets, res, ok := r.serviceTargets(ctx, in.Datasource, in.Signal)
	if !ok {
		return res, nil
	}
	sources := make([]serviceSourceOut, len(targets))
	var group errgroup.Group
	group.SetLimit(servicesConcurrent)
	for i, target := range targets {
		group.Go(func() error {
			out, err := r.readServices(ctx, target, window, in.Datasource == "")
			sources[i] = out
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return r.servicesFailure(in.Datasource, err)
	}
	if in.Service != "" {
		sources = holding(sources, in.Service)
	}
	return servicesResult(sources, window), nil
}

type serviceTarget struct{ uid, dsType, signal string }

func (r *Registry) serviceTargets(ctx context.Context, uid, signal string) ([]serviceTarget, Result, bool) {
	if uid != "" {
		inst, res, ok := r.instance(ctx, uid)
		if !ok {
			return nil, res, false
		}
		dsType := inst.Datasource.Type()
		if len(datasource.ServiceSignals(dsType)) == 0 {
			return nil, invalid("list_services works on Loki, Tempo, Jaeger, ClickHouse, Elasticsearch and OpenSearch datasources; %s is a %s datasource", clip(uid, maxStepNameBytes), dsType), false
		}
		if signal == "" {
			signal = datasource.ServiceSignals(dsType)[0]
		}
		return []serviceTarget{{uid: inst.UID, dsType: dsType, signal: signal}}, Result{}, true
	}
	var targets []serviceTarget
	for _, inst := range r.deps.Datasources.Visible(ctx) {
		targets = append(targets, targetsOf(inst, signal)...)
	}
	return targets, Result{}, true
}

func targetsOf(inst datasource.Instance, signal string) []serviceTarget {
	dsType := inst.Datasource.Type()
	var targets []serviceTarget
	for _, held := range datasource.ServiceSignals(dsType) {
		if signal == "" || signal == held {
			targets = append(targets, serviceTarget{uid: inst.UID, dsType: dsType, signal: held})
		}
	}
	return targets
}

func (r *Registry) readServices(ctx context.Context, target serviceTarget, window time.Duration, tolerant bool) (serviceSourceOut, error) {
	out := serviceSourceOut{Datasource: target.uid, Type: target.dsType, Signal: target.signal, Services: []string{}}
	got, err := r.deps.Runner.Services(ctx, queryproxy.ServicesRequest{Datasource: target.uid, Signal: target.signal, Window: window})
	switch {
	case err == nil:
		out.Field, out.Services, out.Truncated = got.Field, got.Services, got.Truncated
		return out, nil
	case ctx.Err() != nil, !tolerant, errors.Is(err, queryproxy.ErrInvalidRequest):
		return out, err
	}
	_, code, _ := queryproxy.Classify(err)
	out.Unavailable = code
	return out, nil
}

func (r *Registry) servicesFailure(uid string, err error) (Result, error) {
	if errors.Is(err, queryproxy.ErrInvalidRequest) {
		_, _, message := queryproxy.Classify(err)
		return invalid("%s", message), nil
	}
	return datasourceFailure("Listing services on "+sourceWord(uid), err)
}

func sourceWord(uid string) string {
	if uid == "" {
		return "every source"
	}
	return uid
}

func holding(sources []serviceSourceOut, text string) []serviceSourceOut {
	needle := strings.ToLower(text)
	kept := sources[:0]
	for _, source := range sources {
		matches := slices.DeleteFunc(slices.Clone(source.Services), func(name string) bool {
			return !strings.Contains(strings.ToLower(name), needle)
		})
		if len(matches) > 0 || source.Unavailable != "" {
			source.Services = matches
			kept = append(kept, source)
		}
	}
	return kept
}

func servicesResult(sources []serviceSourceOut, window time.Duration) Result {
	shown, cut := fit(sources, listBudget)
	if window == 0 {
		window = queryproxy.DefaultServicesWindow
	}
	step := fmt.Sprintf("Listed services on %s", plural(len(shown), "source", "sources"))
	return success(step, servicesOut{WindowSeconds: int(window / time.Second), Sources: shown, Truncated: cut})
}
