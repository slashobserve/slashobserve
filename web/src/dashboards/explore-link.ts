import {
  exploreLink,
  exploreText,
  type ExploreKind,
} from "@/datasources/sources"
import { logsDatasourceTypes } from "@/logs/search"
import { metricsDatasourceTypes } from "@/metrics/search"
import { isPanelQuery, queryRef, type DataPanel, type PanelQuery } from "@/spec"

type ExploreTarget = ReturnType<typeof exploreLink>

export type ExploreProblem =
  | "no-query"
  | "no-datasource"
  | "datasources-not-loaded"
  | "no-explorer"
  | "not-a-search"
  | "uses-variables"
  | "clickhouse-traces-query"

export type ExploreRange = { from?: string; to?: string }

export type ExploreOpening = {
  target: ExploreTarget
  label: string
}

const variableReference = /\$(?:[A-Za-z]|\{)/

const spanStoreTypes = ["tempo", "jaeger"]

const searchOnlyTypes = [
  ...spanStoreTypes,
  "elasticsearch",
  "opensearch",
  "clickhouse",
]

function isVisibleQuery(
  query: DataPanel["queries"][number]
): query is PanelQuery {
  return isPanelQuery(query) && query.hide !== true
}

function kindOf(
  panel: DataPanel,
  datasourceType: string | undefined
): ExploreKind | undefined {
  if (panel.type === "logs" || panel.type === "traces") return panel.type
  if (datasourceType === undefined) return undefined
  if ((metricsDatasourceTypes as readonly string[]).includes(datasourceType)) {
    return "metrics"
  }
  if ((logsDatasourceTypes as readonly string[]).includes(datasourceType)) {
    return "logs"
  }
  return undefined
}

function searchFor(
  kind: ExploreKind,
  panel: DataPanel,
  query: PanelQuery,
  text: string,
  range: ExploreRange
) {
  const search = { q: text, ...range }
  if (kind !== "metrics") return search
  const table = panel.type === "table" || query.instant === true
  return table ? { ...search, mode: "table" as const } : search
}

export function exploreTarget(
  panel: DataPanel,
  resolve: (ref: string) => string | undefined,
  datasourceType: ((uid: string) => string | undefined) | undefined,
  range: ExploreRange,
  executed: (ref: string) => string | undefined = () => undefined
): { opening: ExploreOpening } | { problem: ExploreProblem } {
  const index = panel.queries.findIndex(isVisibleQuery)
  const query = panel.queries[index]
  if (query === undefined || !isPanelQuery(query)) {
    return { problem: "no-query" }
  }
  const ref = query.datasource ?? panel.datasource
  const ds = ref === undefined ? undefined : resolve(ref)
  if (ds === undefined) return { problem: "no-datasource" }
  const searchPanel = panel.type === "logs" || panel.type === "traces"
  if (!searchPanel && datasourceType === undefined) {
    return { problem: "datasources-not-loaded" }
  }
  const type = datasourceType?.(ds)
  if (!searchPanel && type !== undefined && searchOnlyTypes.includes(type)) {
    return { problem: "not-a-search" }
  }
  const kind = kindOf(panel, type)
  if (kind === undefined) return { problem: "no-explorer" }
  if (kind === "traces" && type === "clickhouse") {
    return { problem: "clickhouse-traces-query" }
  }
  const text = variableReference.test(query.query)
    ? executed(queryRef(query, index))
    : query.query
  if (text === undefined || text === "") return { problem: "uses-variables" }
  const queries = panel.queries.filter(isVisibleQuery).length
  const page = exploreText[kind].page
  return {
    opening: {
      target: exploreLink(kind, ds, searchFor(kind, panel, query, text, range)),
      label: queries > 1 ? `Open in ${page} (first query)` : `Open in ${page}`,
    },
  }
}

export function exploreProblemText(problem: ExploreProblem): string {
  switch (problem) {
    case "no-query":
      return "no query to open"
    case "datasources-not-loaded":
      return "datasources not loaded"
    case "no-explorer":
      return "no explorer for this datasource"
    case "not-a-search":
      return "its query is not a log or trace search"
    case "no-datasource":
      return "no datasource"
    case "uses-variables":
      return "variables not resolved yet"
    case "clickhouse-traces-query":
      return "SQL trace search is not supported"
  }
}
