package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type mcpCall struct {
	tool string
	args map[string]any
}

func mcpRotation(d dashboardInfo) []mcpCall {
	return []mcpCall{
		{"list_datasources", map[string]any{}},
		{"list_metrics", map[string]any{"datasource": "prom", "search": "http_"}},
		{"query_instant", map[string]any{"datasource": "prom", "query": "sum(rate(http_requests_total[5m]))"}},
		{"get_dashboard", map[string]any{"dashboard_id": d.id}},
	}
}

func rpcBody(payload []byte) []byte {
	text := strings.TrimSpace(string(payload))
	if !strings.HasPrefix(text, "event:") && !strings.HasPrefix(text, "data:") {
		return []byte(text)
	}
	last := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "data:") {
			last = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	return []byte(last)
}

func rpcFailure(payload []byte) (code, message string) {
	var out struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result *struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rpcBody(payload), &out); err != nil {
		return "unreadable", string(payload)
	}
	if out.Error != nil {
		return fmt.Sprintf("rpc_%d", out.Error.Code), out.Error.Message
	}
	if out.Result == nil {
		return "no_result", ""
	}
	if out.Result.IsError {
		text := ""
		if len(out.Result.Content) > 0 {
			text = out.Result.Content[0].Text
		}
		return "tool_error", text
	}
	return "", ""
}

const (
	busyStatus = 503
	busyCode   = "busy"

	serverInFlightCap = 32
)

func shedByServer(r reply) bool {
	return r.status == busyStatus && r.code() == busyCode
}

type mcpRun struct {
	e        *env
	d        dashboardInfo
	rotation []mcpCall
	tools    map[string]*recorder
	rpc      *recorder
	shed     *recorder
	errs     *errorLog
	next     atomic.Int64
}

func (m *mcpRun) call(ctx context.Context, key string, c mcpCall) {
	id := m.next.Add(1)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": c.tool, "arguments": c.args},
	}
	r := m.e.api.post(ctx, key, "/mcp", payload)
	if shedByServer(r) {
		m.shed.Fail(r.status, busyCode)
		m.errs.Add(c.tool, busyCode, r.message())
		return
	}
	track(ctx, r, m.tools[c.tool])
	if ctx.Err() != nil {
		return
	}
	if !r.ok() {
		m.errs.Add(c.tool, r.failureCode(), r.message())
		return
	}
	if code, message := rpcFailure(r.body); code != "" {
		m.rpc.Fail(r.status, code)
		m.errs.Add(c.tool, code, truncate(redact(message)))
	}
}

func (m *mcpRun) worker(ctx context.Context, client, slot int) {
	cfg := m.e.cfg
	offset := time.Duration(client*cfg.mcpInFlight+slot) * cfg.ramp / time.Duration(cfg.mcpClients*cfg.mcpInFlight)
	if !m.e.sleep(ctx, offset) {
		return
	}
	key := m.e.editorKey(client)
	for n := client + slot; ctx.Err() == nil; n++ {
		m.call(ctx, key, m.rotation[n%len(m.rotation)])
	}
}

func runMCP(ctx context.Context, e *env) (result, error) {
	d, err := e.dashboard(ctx)
	if err != nil {
		return result{}, err
	}
	m := &mcpRun{e: e, d: d, rotation: mcpRotation(d), tools: map[string]*recorder{}, rpc: newRecorder(), shed: newRecorder(), errs: newErrorLog()}
	for _, c := range m.rotation {
		m.tools[c.tool] = newRecorder()
	}
	pctx, cancel := context.WithTimeout(ctx, e.cfg.ramp+e.cfg.duration)
	defer cancel()
	var wg sync.WaitGroup
	for client := 0; client < e.cfg.mcpClients; client++ {
		for slot := 0; slot < e.cfg.mcpInFlight; slot++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.worker(pctx, client, slot)
			}()
		}
	}
	wg.Wait()
	return m.summarize(), nil
}

func (m *mcpRun) summarize() result {
	var all []*recorder
	sec := section{Title: "MCP tool calls"}
	sec.Metrics = append(sec.Metrics,
		countMetric("clients", float64(m.e.cfg.mcpClients)),
		countMetric("calls in flight per client", float64(m.e.cfg.mcpInFlight)),
	)
	for _, c := range m.rotation {
		rec := m.tools[c.tool]
		all = append(all, rec)
		sec.Metrics = append(sec.Metrics,
			countMetric(c.tool+" calls", float64(rec.Total())),
			durationMetric(c.tool+" p50", rec.Quantile(0.5)),
			durationMetric(c.tool+" p95", rec.Quantile(0.95)),
		)
	}
	merged := merge(all...)
	answered, shed := merged.Total(), m.shed.Failures()
	sec.Metrics = append(sec.Metrics, failureMetrics("mcp", merged)...)
	sec.Metrics = append(sec.Metrics,
		countMetric("mcp shed by the server, 503 busy", float64(shed)),
		percentMetric("mcp shed share of all requests", share(shed, answered+shed)),
		countMetric("mcp JSON-RPC and tool errors", float64(m.rpc.Failures())),
	)
	sec.Errors = m.errs.Samples()
	codes := m.rpc.Codes()
	names := make([]string, 0, len(codes))
	for code := range codes {
		names = append(names, code)
	}
	sort.Strings(names)
	for _, code := range names {
		sec.Metrics = append(sec.Metrics, countMetric("mcp error "+code, float64(codes[code])))
	}
	gates := append(outcomeGates("mcp", merged), gate{
		Name:   "no JSON-RPC or tool errors in mcp",
		Pass:   m.rpc.Failures() == 0,
		Detail: fmt.Sprintf("%d errors", m.rpc.Failures()),
	})
	gates = append(gates, m.shedGate(shed))
	return result{sections: []section{sec}, gates: gates}
}

func (m *mcpRun) shedGate(shed int) gate {
	servers := max(1, len(m.e.cfg.replicas))
	offered := (m.e.cfg.mcpClients*m.e.cfg.mcpInFlight + servers - 1) / servers
	name := "no mcp call shed while each server's share fits its cap"
	if offered > serverInFlightCap {
		return gate{Name: name, Pass: true, Detail: fmt.Sprintf("%d calls in flight per server exceed the cap of %d; %d shed is expected", offered, serverInFlightCap, shed)}
	}
	return gate{Name: name, Pass: shed == 0, Detail: fmt.Sprintf("%d calls in flight per server, cap %d; %d shed", offered, serverInFlightCap, shed)}
}

func share(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}
