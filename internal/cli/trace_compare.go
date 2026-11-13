package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultCompareLimit = 20
	maxCompareLimit     = 100
)

type traceCompareRequest struct {
	Selected []string `json:"selected"`
	Baseline []string `json:"baseline"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Limit    int      `json:"limit"`
}

type traceCompareSet struct {
	Requested int `json:"requested"`
	Read      int `json:"read"`
}

type traceCompareBody struct {
	Selected    traceCompareSet `json:"selected"`
	Baseline    traceCompareSet `json:"baseline"`
	Partial     bool            `json:"partial"`
	Truncated   bool            `json:"truncated"`
	Differences []struct {
		Attribute     string  `json:"attribute"`
		Value         string  `json:"value"`
		SelectedShare float64 `json:"selected_share"`
		BaselineShare float64 `json:"baseline_share"`
	} `json:"differences"`
}

func (a *app) compareTraces(cmd *cobra.Command, args []string) error {
	opts := a.traces
	switch {
	case len(args) > 0:
		return usagef("%s takes trace ids from --compare and --baseline, not as arguments", cmd.CommandPath())
	case opts.ask != "" || opts.dryRun:
		return usagef("--compare cannot be used with --ask or --dry-run")
	case len(opts.baseline) == 0:
		return usagef("--compare needs --baseline <id,...>")
	case opts.limit < 1 || opts.limit > maxCompareLimit:
		return usagef("--limit must be 1 to %d", maxCompareLimit)
	case opts.datasource == "":
		return usagef("%s needs --datasource <uid>; observe doctor lists them", cmd.CommandPath())
	}
	selected, err := traceIDs("--compare", opts.compare)
	if err != nil {
		return err
	}
	baseline, err := traceIDs("--baseline", opts.baseline)
	if err != nil {
		return err
	}
	c, err := a.newClient()
	if err != nil {
		return err
	}
	data, err := c.post(cmd.Context(), "datasources/"+url.PathEscape(opts.datasource)+"/trace-compare", traceCompareRequest{Selected: selected, Baseline: baseline, From: opts.from, To: opts.to, Limit: opts.limit})
	if err != nil {
		return err
	}
	out := a.output()
	if a.json {
		out.body(data)
		return nil
	}
	return printTraceCompare(out, data)
}

func traceIDs(flag string, raw []string) ([]string, error) {
	ids := make([]string, 0, len(raw))
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if !traceIDPattern.MatchString(id) {
			return nil, usagef("%q in %s is not a trace id; a trace id is 16 or 32 hexadecimal characters", text(id), flag)
		}
		ids = append(ids, strings.ToLower(id))
	}
	if len(ids) == 0 {
		return nil, usagef("%s needs at least one trace id", flag)
	}
	return ids, nil
}

func printTraceCompare(out *output, data []byte) error {
	compared, err := decode[traceCompareBody](data)
	if err != nil {
		return err
	}
	if len(compared.Differences) == 0 {
		out.notice("No attribute value differs by 10 percentage points or more between the two sets")
	} else {
		rows := make([][]string, 0, len(compared.Differences))
		for _, d := range compared.Differences {
			rows = append(rows, []string{text(d.Attribute), text(d.Value), sharePercent(d.SelectedShare), sharePercent(d.BaselineShare)})
		}
		out.table([]string{"ATTRIBUTE", "VALUE", "SELECTED", "BASELINE"}, rows)
	}
	if compared.Partial {
		out.notice("Read %d of %d selected traces and %d of %d baseline traces; the rest were not found in the range", compared.Selected.Read, compared.Selected.Requested, compared.Baseline.Read, compared.Baseline.Requested)
	}
	if compared.Truncated {
		out.notice("More differences exist than are shown; raise --limit, up to %d", maxCompareLimit)
	}
	return nil
}

func sharePercent(share float64) string {
	return fmt.Sprintf("%.1f%%", share*100)
}
