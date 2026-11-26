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
  "no-explorer" | "no-datasource" | "uses-variables" | "clickhouse-traces-query"

export type ExploreRange = { from?: string; to?: string }

export type ExploreOpening = {
  target: ExploreTarget
  label: string
}

const variableReference = /\$(?:[A-Za-z]|\{)/

const spanStoreTypes = ["tempo", "jaeger"]

function kindOf(
  panel: DataPanel,
  datasourceType: string | undefined
): ExploreKind | undefined {
  if (panel.type === "logs" || panel.type === "traces") return panel.type
  if (datasourceType === undefined) return undefined
  if ((metricsDatasourceTypes as readonly string[]).includes(datasourceType)) {
    return "metrics"
  }
  if (spanStoreTypes.includes(datasourceType)) return "traces"
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
  datasourceType: (uid: string) => string | undefined,
  range: ExploreRange,
  executed: (ref: string) => string | undefined = () => undefined
): { opening: ExploreOpening } | { problem: ExploreProblem } {
  const index = panel.queries.findIndex(
    (query) => isPanelQuery(query) && query.hide !== true
  )
  const query = panel.queries[index]
  if (query === undefined || !isPanelQuery(query)) {
    return { problem: "no-datasource" }
  }
  const ref = query.datasource ?? panel.datasource
  const ds = ref === undefined ? undefined : resolve(ref)
  if (ds === undefined) return { problem: "no-datasource" }
  const type = datasourceType(ds)
  const kind = kindOf(panel, type)
  if (kind === undefined) return { problem: "no-explorer" }
  if (kind === "traces" && type === "clickhouse") {
    return { problem: "clickhouse-traces-query" }
  }
  const text = variableReference.test(query.query)
    ? executed(queryRef(query, index))
    : query.query
  if (text === undefined || text === "") return { problem: "uses-variables" }
  const queries = panel.queries.filter(isPanelQuery).length
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
    case "no-explorer":
      return "no explorer for this datasource"
    case "no-datasource":
      return "no datasource"
    case "uses-variables":
      return "variables not resolved yet"
    case "clickhouse-traces-query":
      return "SQL trace search is not supported"
  }
}
