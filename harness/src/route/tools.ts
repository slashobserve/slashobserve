import type { AgentSettings } from "../protocol"
import { full, type Task } from "./route"

export const listMoreTools = "list_more_tools"

export const stepAllTools =
  "Offered every investigation tool for the rest of the run"

const taskBasics = [
  "list_services",
  "list_metrics",
  "list_labels",
  "list_label_values",
  "save_finding",
  "note_hypotheses",
  "list_mcp_sources",
  "find_mcp_tools",
  "call_mcp_tool",
  listMoreTools,
]

const taskTools: Record<Exclude<Task, "">, readonly string[]> = {
  measure: [
    "measure_window",
    "measure_aggregate",
    "query_range",
    "query_instant",
  ],
  logs: [
    "query_range",
    "measure_aggregate",
    "log_patterns",
    "log_facets",
    "log_context",
    "sample_rows",
  ],
  traces: [
    "trace_stats",
    "trace_error_origins",
    "trace_slow_path",
    "get_trace",
    "compare_traces",
    "logs_for_trace",
  ],
  incident: [
    "list_annotations",
    "measure_window",
    "query_range",
    "trace_stats",
    "trace_error_origins",
    "trace_slow_path",
    "log_patterns",
    "get_trace",
  ],
  inventory: [
    "list_datasources",
    "list_dashboards",
    "list_observations",
    "search_workspace",
  ],
  explain: [
    "get_dashboard",
    "get_panels",
    "get_observation",
    "list_dashboards",
    "search_workspace",
    "get_spec_guide",
    "query_range",
  ],
}

export type Subject = { dashboard: boolean; observation: boolean }

function subjectTools(subject: Subject): string[] {
  const names: string[] = []
  if (subject.dashboard) {
    names.push("get_panels")
  }
  if (subject.observation) {
    names.push("get_observation")
  }
  return names
}

export function toolsForTask(
  task: Task,
  all: readonly string[],
  subject: Subject
): string[] {
  if (task === full) {
    return all.filter((name) => name !== listMoreTools)
  }
  const kept = new Set([
    ...taskBasics,
    ...taskTools[task],
    ...subjectTools(subject),
  ])
  return all.filter((name) => kept.has(name))
}

export function callsForTask(task: Task, agent: AgentSettings): number {
  if (agent.investigation_calls > 0) {
    return agent.investigation_calls
  }
  return agent.task_calls[task] ?? agent.default_calls
}
