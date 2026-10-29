package grafana

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	legacyAlertListReason  = "Legacy Grafana alerts are not available in SlashObserve."
	managedAlertListReason = "Grafana-managed alerts are not available; SlashObserve lists Prometheus rule alerts."
	alertSourceReason      = "An alert list needs a Prometheus datasource."
	alertStateReason       = "Only firing and pending alerts are listed."
	alertLabelFilterReason = "Only a list of label matchers such as severity=\"critical\" is supported."
	alertLimitReason       = "Only 1 to 100 alerts can be listed."
	alertSortReason        = "This sort order is not supported."
	alertStatReason        = "The colors of the stat view are not supported yet."
	maxAlertItems          = 100
)

var (
	alertMatcher    = `[A-Za-z_][A-Za-z0-9_]*\s*(?:=|!=|=~|!~)\s*"(?:[^"\\]|\\.)*"`
	alertMatcherSet = regexp.MustCompile(`^` + alertMatcher + `(?:\s*,\s*` + alertMatcher + `)*$`)

	alertSortOrders = map[int]string{1: "alphaAsc", 2: "alphaDesc", 3: "importance", 4: "timeAsc", 5: "timeDesc"}

	alertStatThresholdsDefault = map[string]any{
		"mode": "absolute",
		"steps": []any{
			map[string]any{"color": "green", "value": nil},
			map[string]any{"color": "red", "value": 80},
		},
	}
)

func alertListDatasource(raw map[string]any) any {
	v := lookupValue(raw, "options.datasource")
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return v
}

func alertListReason(n *panelNode) string {
	if spec.PanelType(n.typ) != spec.AlertList {
		return ""
	}
	options, _ := asObject(n.raw["options"])
	_, legacyFilter := options["stateFilter"].([]any)
	if present(options["show"]) || legacyFilter {
		return legacyAlertListReason
	}
	if len(n.ds.targets) == 0 || n.ds.targets[0].kind == refSpecial {
		return managedAlertListReason
	}
	if ref := n.ds.targets[0]; ref.supported() && ref.src.kind != kindPrometheus {
		return alertSourceReason
	}
	return ""
}

func (r *registry) scanAlertList(n *panelNode) {
	ref := r.classify(alertListDatasource(n.raw))
	if ref.kind == refNone {
		return
	}
	n.ds.targets = []dsRef{ref}
	if ref.src == nil {
		return
	}
	ref.src.panels++
	if len(ref.src.samples) < maxSamples {
		ref.src.samples = append(ref.src.samples, map[string]any{"expr": "ALERTS"})
	}
}

func (b *panelBuild) alertListQueries() {
	b.consume("options.datasource")
	b.built = []*builtQuery{{q: spec.Query{Query: b.alertsExpression()}, src: b.n.ds.targets[0].src}}
}

func (b *panelBuild) alertsExpression() string {
	matchers := []string{b.alertStateMatcher()}
	if filter := b.alertLabelMatchers(); filter != "" {
		matchers = append(matchers, filter)
	}
	if name := strings.TrimSpace(asString(lookupValue(b.raw, "options.alertName"))); name != "" {
		matchers = append(matchers, fmt.Sprintf(`alertname=~"(?i).*%s.*"`, promQuoted(regexp.QuoteMeta(name))))
	}
	b.consume("options.alertName")
	return "ALERTS{" + strings.Join(matchers, ",") + "}"
}

func promQuoted(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func (b *panelBuild) alertStateMatcher() string {
	filter, _ := asObject(lookupValue(b.raw, "options.stateFilter"))
	b.consume("options.stateFilter")
	kept := map[string]bool{}
	for _, state := range []string{"firing", "pending"} {
		flag, set := filter[state]
		kept[state] = !set || truthy(flag)
	}
	for _, state := range []string{"noData", "normal", "inactive"} {
		if truthy(filter[state]) {
			b.partial("options.stateFilter."+state, alertStateReason)
		}
	}
	switch {
	case kept["firing"] && !kept["pending"]:
		return `alertstate="firing"`
	case kept["pending"] && !kept["firing"]:
		return `alertstate="pending"`
	case !kept["firing"] && !kept["pending"]:
		b.partial("options.stateFilter", alertStateReason)
	}
	return `alertstate=~"firing|pending"`
}

func (b *panelBuild) alertLabelMatchers() string {
	path := "options.alertInstanceLabelFilter"
	filter := strings.TrimSpace(asString(lookupValue(b.raw, path)))
	b.consume(path)
	if inner, ok := strings.CutSuffix(strings.TrimPrefix(filter, "{"), "}"); ok && strings.HasPrefix(filter, "{") {
		filter = strings.TrimSpace(inner)
	}
	if filter == "" {
		return ""
	}
	if !alertMatcherSet.MatchString(filter) {
		b.partial(path, alertLabelFilterReason)
		return ""
	}
	return filter
}

func displayAlertList(b *panelBuild) {
	d := b.disp()
	if v, ok := b.take("options.maxItems"); ok {
		if n, isInt := asInt(v); isInt && n >= 1 && n <= maxAlertItems {
			d.Limit = n
		} else {
			b.partial("options.maxItems", alertLimitReason)
		}
	}
	if v, ok := b.take("options.sortOrder"); ok {
		n, _ := asInt(v)
		if sort, known := alertSortOrders[n]; known {
			d.Sort = sort
		} else {
			b.partial("options.sortOrder", alertSortReason)
		}
	}
	d.ViewMode = b.enumOption("options.viewMode", "list", "stat")
	b.alertStatOptions()
}

func (b *panelBuild) alertStatOptions() {
	if v, ok := b.take("options.statThresholds"); ok && !sameJSON(v, alertStatThresholdsDefault) {
		b.partial("options.statThresholds", alertStatReason)
	}
}
