import type { ToolSpec } from "./session/types"

export const saveTools: ReadonlySet<string> = new Set([
  "create_dashboard",
  "update_dashboard",
  "update_panels",
  "import_grafana_dashboard",
  "save_finding",
])

export const queryTools: ReadonlySet<string> = new Set([
  "query_range",
  "query_instant",
  "measure_window",
  "measure_aggregate",
])

const dataReadTools: ReadonlySet<string> = new Set([
  "trace_stats",
  "sample_rows",
  "get_trace",
  "compare_traces",
  "trace_error_origins",
  "trace_slow_path",
  "log_context",
  "log_patterns",
  "log_facets",
  "logs_for_trace",
  "call_mcp_tool",
])

export const sweepTools: ReadonlySet<string> = new Set(["read_series"])

export function isSave(tool: string): boolean {
  return saveTools.has(tool)
}

export function readsData(tool: string): boolean {
  return queryTools.has(tool) || dataReadTools.has(tool)
}

export function offersWrite(specs: readonly ToolSpec[]): boolean {
  return specs.some((spec) => spec.readOnly === false && !isSave(spec.name))
}
