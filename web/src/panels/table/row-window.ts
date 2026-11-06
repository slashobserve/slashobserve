export type RowWindow = {
  start: number
  end: number
  before: number
  after: number
}

export const windowAbove = 100

export function rowWindow(input: {
  scrollTop: number
  viewport: number
  rowHeight: number
  count: number
  overscan: number
}): RowWindow {
  const { scrollTop, viewport, rowHeight, count, overscan } = input
  if (rowHeight <= 0) return { start: 0, end: count, before: 0, after: 0 }
  const clamp = (index: number) => Math.min(Math.max(0, index), count)
  const first = Math.floor(Math.max(0, scrollTop) / rowHeight)
  const last = Math.ceil((Math.max(0, scrollTop) + viewport) / rowHeight)
  const start = clamp(first - overscan)
  const end = Math.max(start, clamp(last + overscan))
  return {
    start,
    end,
    before: start * rowHeight,
    after: (count - end) * rowHeight,
  }
}

export function windows(input: {
  paginate: boolean
  count: number
  expandedCount: number
  grows: boolean
  columnsFixed: boolean
}): boolean {
  return (
    !input.paginate &&
    input.count > windowAbove &&
    input.expandedCount === 0 &&
    !input.grows &&
    input.columnsFixed
  )
}
