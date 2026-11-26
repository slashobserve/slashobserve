import { legendNames } from "./legend-model"

export type TextMeasure = (text: string, weight: 400 | 500) => number

const plainMinPx = 160
const plainMaxPx = 280
const plainShare = 0.3
const tableMaxShare = 0.5
const chartMinPx = 120
const containerPadRem = 0.5
const cellPadRem = 0.75
const swatchRem = 1
const swatchGapRem = 0.375
const nameMinRem = 4

function widest(
  texts: readonly string[],
  weight: 400 | 500,
  measure: TextMeasure
): number {
  return Math.max(0, ...texts.map((text) => measure(text, weight)))
}

export function legendNameMinWidth(rem: number): number {
  return nameMinRem * rem
}

export function legendNameRoom(
  width: number,
  rem: number,
  valuesPx: number
): number {
  return (
    width -
    (containerPadRem + cellPadRem + swatchRem + swatchGapRem) * rem -
    valuesPx
  )
}

export function inlineValuesWidth(
  values: readonly string[],
  measure: TextMeasure,
  rem: number
): number {
  return widest(values, 400, measure) + swatchGapRem * rem
}

export function legendValueWidths(
  headers: readonly string[],
  cells: readonly (readonly string[])[],
  measure: TextMeasure,
  rem: number
): number[] {
  return headers.map(
    (header, column) =>
      cellPadRem * rem +
      Math.max(
        measure(header, 500),
        widest(
          cells.map((row) => row[column]),
          400,
          measure
        )
      )
  )
}

export function legendTableWidth(
  labels: readonly string[],
  headers: readonly string[],
  cells: readonly (readonly string[])[],
  measure: TextMeasure,
  rem: number
): number {
  const name = Math.max(
    legendNameMinWidth(rem),
    (cellPadRem + swatchRem + swatchGapRem) * rem + widest(labels, 400, measure)
  )
  const values = legendValueWidths(headers, cells, measure, rem).reduce(
    (sum, width) => sum + width,
    0
  )
  return Math.ceil(containerPadRem * rem + name + values)
}

export function legendListWidth(
  labels: readonly string[],
  values: readonly string[],
  measure: TextMeasure,
  rem: number,
  panelWidth: number
): number | null {
  const shortest = legendNames(labels, {
    px: 0,
    measure: (text) => measure(text, 400),
  })
  for (const names of [labels, shortest]) {
    const natural = Math.ceil(
      (containerPadRem + cellPadRem + swatchRem + swatchGapRem) * rem +
        widest(names, 400, measure) +
        inlineValuesWidth(values, measure, rem)
    )
    if (natural + chartMinPx <= panelWidth) return natural
  }
  return null
}

export function rightLegendWidth(
  tableNaturalPx: number | null,
  panelWidth: number,
  configuredPx?: number,
  listNaturalPx: number | null = null
): number {
  if (configuredPx !== undefined) {
    return Math.min(configuredPx, Math.max(0, panelWidth - chartMinPx))
  }
  if (tableNaturalPx !== null) {
    return Math.min(tableNaturalPx, panelWidth * tableMaxShare)
  }
  const plain = Math.min(
    plainMaxPx,
    Math.max(plainMinPx, panelWidth * plainShare)
  )
  if (listNaturalPx === null) return plain
  return Math.max(plain, Math.min(listNaturalPx, panelWidth - chartMinPx))
}
