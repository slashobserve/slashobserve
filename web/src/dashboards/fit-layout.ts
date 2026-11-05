import type { Layout } from "react-grid-layout"

export const gridColumns = 24
export const gridMargin = 10

type Item = Layout[number]

export function pixelWidth(containerWidth: number, w: number): number {
  const column = (containerWidth - (gridColumns - 1) * gridMargin) / gridColumns
  return column * w + (w - 1) * gridMargin
}

export function columnsFor(
  minPixels: number | undefined,
  containerWidth: number
): number {
  if (minPixels === undefined || !(minPixels > 0)) return 1
  return Math.min(
    gridColumns,
    Math.max(
      1,
      Math.ceil(
        (minPixels + gridMargin) / (pixelWidth(containerWidth, 1) + gridMargin)
      )
    )
  )
}

type Placed = { item: Item; width: number; fixed: boolean }

function bandsOf(sorted: readonly Item[]): Item[][] {
  const bands: Item[][] = []
  let bottom = 0
  for (const item of sorted) {
    const current = bands.at(-1)
    if (current !== undefined && item.y < bottom) {
      current.push(item)
      bottom = Math.max(bottom, item.y + item.h)
    } else {
      bands.push([item])
      bottom = item.y + item.h
    }
  }
  return bands
}

function lineCount(widths: readonly number[]): number {
  let lines = 1
  let used = 0
  for (const width of widths) {
    if (used > 0 && used + width > gridColumns) {
      lines++
      used = 0
    }
    used += width
  }
  return lines
}

function splitLines(placed: readonly Placed[]): Placed[][] {
  const perLine = Math.ceil(
    placed.length / lineCount(placed.map(({ width }) => width))
  )
  const lines: Placed[][] = []
  let used = 0
  for (const entry of placed) {
    const line = lines.at(-1)
    if (
      line === undefined ||
      used + entry.width > gridColumns ||
      line.length >= perLine
    ) {
      lines.push([entry])
      used = entry.width
    } else {
      line.push(entry)
      used += entry.width
    }
  }
  return lines
}

function widenLine(line: readonly Placed[]): number[] {
  const widths = line.map(({ width }) => width)
  const room = gridColumns - widths.reduce((sum, width) => sum + width, 0)
  const responsive = line.flatMap((entry, index) =>
    entry.fixed ? [] : [index]
  )
  if (room <= 0 || responsive.length === 0) return widths
  const responsiveSum = responsive.reduce(
    (sum, index) => sum + widths[index],
    0
  )
  const grown = [...widths]
  let given = 0
  for (const index of responsive) {
    const gain = Math.floor((widths[index] * room) / responsiveSum)
    grown[index] += gain
    given += gain
  }
  for (const index of responsive.slice(0, room - given)) grown[index] += 1
  return grown
}

export function fitLayout(
  layout: Layout,
  minimum: ReadonlyMap<string, number>,
  fixed: ReadonlySet<string>
): Layout {
  const wanted = (item: Item): number =>
    fixed.has(item.i)
      ? item.w
      : Math.min(gridColumns, Math.max(1, minimum.get(item.i) ?? 1))

  const sorted = [...layout].sort((a, b) => a.y - b.y || a.x - b.x)
  const fitted = new Map<string, Item>()
  let dy = 0

  for (const band of bandsOf(sorted)) {
    const top = band[0].y
    const bottom = Math.max(...band.map((item) => item.y + item.h))
    const narrow = band.some((item) => item.w < wanted(item))

    if (!narrow) {
      for (const item of band) {
        if (dy !== 0) fitted.set(item.i, { ...item, y: item.y + dy })
      }
      continue
    }

    const placed: Placed[] = band.map((item) => ({
      item,
      width: Math.max(item.w, wanted(item)),
      fixed: fixed.has(item.i),
    }))
    let lineTop = top + dy
    for (const line of splitLines(placed)) {
      const widths = widenLine(line)
      let x = 0
      for (const [index, entry] of line.entries()) {
        fitted.set(entry.item.i, {
          ...entry.item,
          x,
          y: lineTop,
          w: widths[index],
        })
        x += widths[index]
      }
      lineTop += Math.max(...line.map(({ item }) => item.h))
    }
    dy += lineTop - (top + dy) - (bottom - top)
  }

  return layout.map((item) => {
    const next = fitted.get(item.i)
    return next !== undefined &&
      (next.x !== item.x || next.y !== item.y || next.w !== item.w)
      ? next
      : item
  })
}
