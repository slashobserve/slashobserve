import { isDashboardQuery, type DataPanel } from "@/spec"

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
