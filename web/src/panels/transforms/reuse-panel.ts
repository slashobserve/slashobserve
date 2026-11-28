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
  const reused: DataPanel = { ...panel, queries: source.queries }
  delete reused.datasource
  delete reused.maxDataPoints
  delete reused.interval
  delete reused.timeFrom
  delete reused.timeShift
  delete reused.transform
  const { datasource, maxDataPoints, interval, timeFrom, timeShift } = source
  return {
    ...reused,
    ...(datasource === undefined ? {} : { datasource }),
    ...(maxDataPoints === undefined ? {} : { maxDataPoints }),
    ...(interval === undefined ? {} : { interval }),
    ...(timeFrom === undefined ? {} : { timeFrom }),
    ...(timeShift === undefined ? {} : { timeShift }),
    ...(transform.length > 0 ? { transform } : {}),
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
  const scopes = adhocFiltersOf(variables, selections)
  if (scopes.length === 0) return undefined
  const after = query.source.withTransforms
    ? (source.transform ?? []).length
    : 0
  return { scopes, after }
}
