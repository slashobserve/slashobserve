package cli

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const servicesConcurrency = 4

type servicesBody struct {
	Datasource    string   `json:"datasource"`
	Signal        string   `json:"signal"`
	Field         string   `json:"field"`
	WindowSeconds int      `json:"window_seconds"`
	Services      []string `json:"services"`
	Truncated     bool     `json:"truncated"`
}

type serviceSource struct {
	Datasource  string   `json:"datasource"`
	Type        string   `json:"type"`
	Signal      string   `json:"signal"`
	Field       string   `json:"field,omitempty"`
	Services    []string `json:"services"`
	Truncated   bool     `json:"truncated,omitempty"`
	Unavailable string   `json:"unavailable,omitempty"`
}

type servicesOptions struct {
	signal  string
	window  string
	service string
}

func (o servicesOptions) query(signal string) url.Values {
	query := url.Values{}
	if signal != "" {
		query.Set("signal", signal)
	}
	if o.window != "" {
		query.Set("window", o.window)
	}
	return query
}

func (a *app) servicesCommand() *cobra.Command {
	var opts servicesOptions
	cmd := &cobra.Command{
		Use:   "services [datasource-uid]",
		Short: "List the services a log, trace or metric datasource has seen recently, or find which datasources hold one",
		Long:  "services lists the services a datasource has seen over a recent window, one per line, with the label, column or attribute that holds them.\nWithout a datasource uid it asks every Loki, Tempo, Jaeger, ClickHouse, Elasticsearch, OpenSearch and Prometheus datasource and prints one row per source and signal, and a source that cannot answer in time says why while the others still answer; ClickHouse answers once for logs and once for traces.\n--service keeps only the sources that hold a service whose name contains the text, case-insensitive, so `observe datasources services --service checkout` prints which sources hold checkout.\n--signal picks logs, traces or metrics, and a Prometheus datasource lists the values of the first of service_name, service, app and job that it has, which is the label a metric selector uses.\n--window (such as 15m, default 1h, at most 6h) sets how far back the window ends now, except on Jaeger, which lists every service it retains.\nThe server caches an answer for about a minute and lists at most 200 services per source.",
		Args:  rangeArgs(0, 1, "at most one datasource uid"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return a.runServicesOne(cmd, args[0], opts)
			}
			return a.runServicesAll(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.signal, "signal", "", "logs, traces or metrics; default is the datasource's own signal, and both for ClickHouse without a uid")
	cmd.Flags().StringVar(&opts.window, "window", "", "how far back to look, such as 15m or 1h (default 1h, at most 6h)")
	cmd.Flags().StringVar(&opts.service, "service", "", "keep only services whose name contains this text, case-insensitive")
	return cmd
}

func (a *app) runServicesOne(cmd *cobra.Command, uid string, opts servicesOptions) error {
	data, err := a.getDiscovery(cmd, uid, "services", opts.query(opts.signal))
	if err != nil {
		return err
	}
	out := a.output()
	return render(out, data, func(body servicesBody) error {
		for _, name := range matching(body.Services, opts.service) {
			out.line("%s", text(name))
		}
		if body.Truncated {
			out.notice("The list is cut at %d services; narrow it with --service.", len(body.Services))
		}
		if body.Field != "" {
			out.notice("Services are in %s over the last %s.", text(body.Field), windowText(body.WindowSeconds))
		}
		return nil
	})
}

func (a *app) runServicesAll(cmd *cobra.Command, opts servicesOptions) error {
	c, err := a.newClient()
	if err != nil {
		return err
	}
	list, err := listDatasources(cmd.Context(), c)
	if err != nil {
		return err
	}
	targets := serviceTargets(list, opts.signal)
	sources := make([]serviceSource, len(targets))
	var group errgroup.Group
	group.SetLimit(servicesConcurrency)
	for i, target := range targets {
		group.Go(func() error {
			sources[i] = readServiceSource(cmd.Context(), c, target, opts)
			return nil
		})
	}
	_ = group.Wait()
	if err := cmd.Context().Err(); err != nil {
		return err
	}
	if opts.service != "" {
		sources = holdingService(sources, opts.service)
	}
	out := a.output()
	if a.json {
		return printServicesJSON(out, sources)
	}
	if len(sources) == 0 {
		out.notice("No datasource holds a matching service")
		return nil
	}
	rows := make([][]string, len(sources))
	for i, s := range sources {
		rows[i] = []string{text(s.Datasource), text(s.Type), s.Signal, text(s.Field), serviceCell(s)}
	}
	out.table([]string{"DATASOURCE", "TYPE", "SIGNAL", "FIELD", "SERVICES"}, rows)
	return nil
}

type serviceTarget struct{ uid, dsType, signal string }

func serviceTargets(list []datasourceEntry, signal string) []serviceTarget {
	var targets []serviceTarget
	for _, ds := range list {
		signals := datasource.ServiceSignals(ds.Type)
		if ds.Spans != "" {
			signals = []string{datasource.SignalTraces}
		}
		for _, held := range signals {
			if signal == "" || signal == held {
				targets = append(targets, serviceTarget{uid: ds.UID, dsType: ds.Type, signal: held})
			}
		}
	}
	return targets
}

func readServiceSource(ctx context.Context, c *client, target serviceTarget, opts servicesOptions) serviceSource {
	source := serviceSource{Datasource: target.uid, Type: target.dsType, Signal: target.signal, Services: []string{}}
	data, err := c.get(ctx, "datasources/"+url.PathEscape(target.uid)+"/services", opts.query(target.signal))
	if err != nil {
		source.Unavailable = err.Error()
		return source
	}
	body, err := decode[servicesBody](data)
	if err != nil {
		source.Unavailable = err.Error()
		return source
	}
	source.Field, source.Services, source.Truncated = body.Field, body.Services, body.Truncated
	return source
}

func holdingService(sources []serviceSource, needle string) []serviceSource {
	var kept []serviceSource
	for _, source := range sources {
		matches := matching(source.Services, needle)
		if len(matches) > 0 || source.Unavailable != "" {
			source.Services = matches
			kept = append(kept, source)
		}
	}
	return kept
}

func matching(names []string, needle string) []string {
	if needle == "" {
		return names
	}
	needle = strings.ToLower(needle)
	var kept []string
	for _, name := range names {
		if strings.Contains(strings.ToLower(name), needle) {
			kept = append(kept, name)
		}
	}
	return kept
}

const servicesShown = 8

func serviceCell(s serviceSource) string {
	switch {
	case s.Unavailable != "":
		return "unavailable: " + text(s.Unavailable)
	case len(s.Services) == 0:
		return "none seen"
	case len(s.Services) <= servicesShown:
		return text(strings.Join(s.Services, ", "))
	}
	return text(strings.Join(s.Services[:servicesShown], ", ")) + " +" + strconv.Itoa(len(s.Services)-servicesShown) + " more"
}

func windowText(seconds int) string {
	switch {
	case seconds%3600 == 0:
		return strconv.Itoa(seconds/3600) + "h"
	case seconds%60 == 0:
		return strconv.Itoa(seconds/60) + "m"
	}
	return strconv.Itoa(seconds) + "s"
}

func printServicesJSON(out *output, sources []serviceSource) error {
	if sources == nil {
		sources = []serviceSource{}
	}
	data, err := json.Marshal(struct {
		Sources []serviceSource `json:"sources"`
	}{sources})
	if err != nil {
		return err
	}
	out.body(data)
	return nil
}
