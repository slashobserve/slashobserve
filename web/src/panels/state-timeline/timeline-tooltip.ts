import { tooltipRows, type TooltipSort } from "../timeseries/tooltip-rows"
import { segmentAt, type Segment, type TimelineRow } from "./timeline-model"

export const maxTooltipEntries = 10

export type TooltipEntry = { row: number; name: string; segment: Segment }

export type TooltipView = { sort: TooltipSort; hideZeros: boolean }

export type TooltipEntries = { entries: TooltipEntry[]; total: number }

export function tooltipEntries(
  rows: readonly TimelineRow[],
  at: number,
  { sort, hideZeros }: TooltipView
): TooltipEntries {
  const found = rows.flatMap((row, index): TooltipEntry[] => {
    const segment = segmentAt(row.segments, at)
    return segment ? [{ row: index, name: row.name, segment }] : []
  })
  const numeric = found.flatMap((entry, index) =>
    entry.segment.value === null ? [] : [{ index, value: entry.segment.value }]
  )
  const sorted = tooltipRows(
    numeric,
    { sort, hideZeros, cumulative: false, stack: [] },
    numeric.length
  ).rows.map(({ index }) => found[index])
  const textual = found.filter(({ segment }) => segment.value === null)
  const all = [...sorted, ...textual]
  return { entries: all.slice(0, maxTooltipEntries), total: all.length }
}
