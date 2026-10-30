import type { Selections } from "@/api/keys"
import { isDashboardQuery, type DataPanel, type Variable } from "@/spec"

import { adhocFiltersOf, type ReusedFilter } from "./ad-hoc-filter"

export function reusePanel(
  panel: DataPanel,
  source: DataPanel | undefined
): DataPanel {
  const query = panel.queries.find(isDashboardQuery)
  if (!query || !source) return panel
  const sourceTransforms = query.source.withTransforms
    ? (source.transform ?? [])
    : []
  const transform = [...sourceTransforms, ...(panel.transform ?? [])]
  return {
    ...panel,
    datasource: source.datasource,
    queries: source.queries,
    maxDataPoints: source.maxDataPoints,
    interval: source.interval,
    timeFrom: source.timeFrom,
    timeShift: source.timeShift,
    transform: transform.length > 0 ? transform : undefined,
  }
}

export function reusedFilterOf(
  panel: DataPanel,
  source: DataPanel | undefined,
  variables: readonly Variable[],
  selections: Selections
): ReusedFilter | undefined {
  const query = panel.queries.find(isDashboardQuery)
  if (!query?.source.adHocFilters || !source) return undefined
  const filters = adhocFiltersOf(variables, selections)
  if (filters.length === 0) return undefined
  const after = query.source.withTransforms
    ? (source.transform ?? []).length
    : 0
  return { filters, after }
}
