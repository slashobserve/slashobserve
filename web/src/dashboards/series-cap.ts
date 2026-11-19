export type SeriesCap = { text: string; title: string }

export function seriesCapLabel(
  meta: { series_shown?: number; series_total?: number } | undefined
): SeriesCap | null {
  const { series_shown: shown, series_total: total } = meta ?? {}
  if (shown === undefined || total === undefined || total <= shown) return null
  return {
    text: `Top ${shown} of ${total} series`,
    title: `Showing the ${shown} series with the highest peaks of ${total}. Narrow the query or add a filter to see the rest.`,
  }
}
