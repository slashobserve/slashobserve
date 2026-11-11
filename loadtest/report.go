package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type metric struct {
	Name   string   `json:"name"`
	Value  *float64 `json:"value"`
	Unit   string   `json:"unit"`
	Target string   `json:"target,omitempty"`
	Result string   `json:"result,omitempty"`
}

type section struct {
	Title   string        `json:"title"`
	Metrics []metric      `json:"metrics"`
	Errors  []errorSample `json:"errors,omitempty"`
}

type gate struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

type result struct {
	sections []section
	gates    []gate
}

type report struct {
	Label    string            `json:"label"`
	Commit   string            `json:"commit"`
	Target   string            `json:"target"`
	Started  time.Time         `json:"started"`
	Ended    time.Time         `json:"ended"`
	Flags    map[string]string `json:"flags"`
	Sections []section         `json:"sections"`
	Gates    []gate            `json:"gates"`
}

func number(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func durationMetric(name string, d time.Duration) metric {
	return metric{Name: name, Value: number(float64(d) / float64(time.Millisecond)), Unit: "ms"}
}

func secondsMetric(name string, s float64) metric {
	return metric{Name: name, Value: number(s * 1000), Unit: "ms"}
}

func countMetric(name string, n float64) metric {
	return metric{Name: name, Value: number(n), Unit: "count"}
}

func percentMetric(name string, p float64) metric {
	return metric{Name: name, Value: number(p), Unit: "percent"}
}

func bytesMetric(name string, b float64) metric {
	return metric{Name: name, Value: number(b), Unit: "bytes"}
}

func (m metric) under(limit float64, text string) metric {
	m.Target = "< " + text
	switch {
	case m.Value == nil:
		m.Result = "no data"
	case *m.Value < limit:
		m.Result = "met"
	default:
		m.Result = "missed"
	}
	return m
}

func (m metric) text() string {
	if m.Value == nil {
		return "n/a"
	}
	v := *m.Value
	switch m.Unit {
	case "ms":
		if v >= 10000 {
			return fmt.Sprintf("%.1f s", v/1000)
		}
		return fmt.Sprintf("%.0f ms", v)
	case "percent":
		return fmt.Sprintf("%.1f%%", v)
	case "bytes":
		return humanBytes(v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

func humanBytes(v float64) string {
	units := []string{"B", "KiB", "MiB", "GiB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f B", v)
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

func latencyMetrics(prefix string, rec *recorder) []metric {
	return []metric{
		durationMetric(prefix+" p50", rec.Quantile(0.5)),
		durationMetric(prefix+" p95", rec.Quantile(0.95)),
		durationMetric(prefix+" p99", rec.Quantile(0.99)),
	}
}

func failureMetrics(prefix string, rec *recorder) []metric {
	out := []metric{
		countMetric(prefix+" requests", float64(rec.Total())),
		countMetric(prefix+" 5xx or transport errors", float64(rec.ServerErrors())),
		countMetric(prefix+" 4xx other than 429", float64(rec.OtherClientErrors())),
		countMetric(prefix+" 429", float64(rec.TooMany())),
	}
	codes := rec.Codes()
	names := make([]string, 0, len(codes))
	for code := range codes {
		names = append(names, code)
	}
	sort.Strings(names)
	for _, code := range names {
		out = append(out, countMetric(fmt.Sprintf("%s error code %s", prefix, code), float64(codes[code])))
	}
	return out
}

func outcomeGates(name string, rec *recorder) []gate {
	total := rec.Total()
	other := rec.OtherClientErrors()
	return []gate{
		{
			Name:   "no 5xx or transport errors in " + name,
			Pass:   rec.ServerErrors() == 0,
			Detail: fmt.Sprintf("%d of %d requests", rec.ServerErrors(), total),
		},
		{
			Name:   "4xx other than 429 under 0.1% in " + name,
			Pass:   float64(other) <= 0.001*float64(total),
			Detail: fmt.Sprintf("%d of %d requests", other, total),
		},
	}
}

func (r report) failed() bool {
	for _, g := range r.Gates {
		if !g.Pass {
			return true
		}
	}
	return false
}

func (r report) gateSummary() string {
	var b strings.Builder
	for _, g := range r.Gates {
		fmt.Fprintf(&b, "%s  %s\n", gateWord(g.Pass), g.Name)
	}
	return b.String()
}

func gateWord(pass bool) string {
	if pass {
		return "pass"
	}
	return "FAIL"
}

func (r report) markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Load test %s\n\n", r.Label)
	fmt.Fprintf(&b, "Target: %s\n\n", r.Target)
	fmt.Fprintf(&b, "Commit: %s\n\n", r.Commit)
	fmt.Fprintf(&b, "Started: %s\n\n", r.Started.Format(time.RFC3339))
	fmt.Fprintf(&b, "Ended: %s\n", r.Ended.Format(time.RFC3339))
	for _, s := range r.Sections {
		fmt.Fprintf(&b, "\n## %s\n\n", s.Title)
		b.WriteString("| Measure | Value | Page target | Result |\n| --- | --- | --- | --- |\n")
		for _, m := range s.Metrics {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", m.Name, m.text(), m.Target, m.Result)
		}
		if len(s.Errors) > 0 {
			b.WriteString("\n| Tool | Error code | Count | Messages |\n| --- | --- | --- | --- |\n")
			for _, e := range s.Errors {
				fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", e.Tool, e.Code, e.Count, strings.ReplaceAll(strings.Join(e.Messages, "; "), "|", "/"))
			}
		}
	}
	b.WriteString("\n## Gates\n\n| Gate | Result | Detail |\n| --- | --- | --- |\n")
	for _, g := range r.Gates {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", g.Name, gateWord(g.Pass), g.Detail)
	}
	return b.String()
}

func writeReport(out string, r report) (string, error) {
	dir := filepath.Join(out, r.Started.UTC().Format("20060102T150405Z")+"-"+r.Label)
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode report.json: %w", err)
	}
	if err := writeFiles(dir, map[string][]byte{"report.json": append(raw, '\n'), "report.md": []byte(r.markdown())}); err != nil {
		return "", err
	}
	return dir, nil
}

//nolint:gosec // the directory is the --out flag
func writeFiles(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create the report directory: %w", err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
