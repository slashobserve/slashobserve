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

export function legendTableWidth(
  labels: readonly string[],
  headers: readonly string[],
  cells: readonly (readonly string[])[],
  measure: TextMeasure,
  rem: number
): number {
  const widest = (texts: readonly string[], weight: 400 | 500) =>
    Math.max(0, ...texts.map((text) => measure(text, weight)))
  const name = Math.max(
    nameMinRem * rem,
    (cellPadRem + swatchRem + swatchGapRem) * rem + widest(labels, 400)
  )
  const values = headers.reduce(
    (sum, header, column) =>
      sum +
      cellPadRem * rem +
      Math.max(
        measure(header, 500),
        widest(
          cells.map((row) => row[column]),
          400
        )
      ),
    0
  )
  return Math.ceil(containerPadRem * rem + name + values)
}

export function rightLegendWidth(
  tableNaturalPx: number | null,
  panelWidth: number,
  configuredPx?: number
): number {
  if (configuredPx !== undefined) {
    return Math.min(configuredPx, Math.max(0, panelWidth - chartMinPx))
  }
  if (tableNaturalPx !== null) {
    return Math.min(tableNaturalPx, panelWidth * tableMaxShare)
  }
  return Math.min(plainMaxPx, Math.max(plainMinPx, panelWidth * plainShare))
}
