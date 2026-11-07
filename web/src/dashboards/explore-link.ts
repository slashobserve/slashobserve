import type { DataPanel, PanelQuery } from "@/spec"

export type ExploreTarget =
  | {
      to: "/logs"
      search: { ds: string; q: string; from?: string; to?: string }
    }
  | {
      to: "/traces"
      search: { ds: string; q?: string; from?: string; to?: string }
    }

export type ExploreProblem =
  | "not-a-logs-or-traces-panel"
  | "several-queries"
  | "no-datasource"
  | "uses-variables"
  | "clickhouse-traces-query"

export type ExploreRange = { from?: string; to?: string }

const variableReference = /\$(?:[A-Za-z]|\{)/

function isPanelQuery(
  query: DataPanel["queries"][number]
): query is PanelQuery {
  return "query" in query
}

export function exploreTarget(
  panel: DataPanel,
  resolve: (ref: string) => string | undefined,
  datasourceType: (uid: string) => string | undefined,
  range: ExploreRange
): { target: ExploreTarget } | { problem: ExploreProblem } {
  if (panel.type !== "logs" && panel.type !== "traces") {
    return { problem: "not-a-logs-or-traces-panel" }
  }
  if (panel.queries.length > 1) return { problem: "several-queries" }
  const query = panel.queries[0]
  if (query === undefined || !isPanelQuery(query)) {
    return { problem: "no-datasource" }
  }
  const ref = query.datasource ?? panel.datasource
  const ds = ref === undefined ? undefined : resolve(ref)
  if (ds === undefined) return { problem: "no-datasource" }
  if (variableReference.test(query.query)) return { problem: "uses-variables" }
  if (panel.type === "traces" && datasourceType(ds) === "clickhouse") {
    return { problem: "clickhouse-traces-query" }
  }
  const search = { ds, q: query.query, ...range }
  return {
    target:
      panel.type === "logs"
        ? { to: "/logs", search }
        : { to: "/traces", search },
  }
}

export function exploreProblemText(problem: ExploreProblem): string {
  switch (problem) {
    case "not-a-logs-or-traces-panel":
      return "not a logs or traces panel"
    case "several-queries":
      return "several queries"
    case "no-datasource":
      return "no datasource"
    case "uses-variables":
      return "uses variables"
    case "clickhouse-traces-query":
      return "SQL trace search is not supported"
  }
}
