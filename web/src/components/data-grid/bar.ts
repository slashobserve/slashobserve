const barMinWidthPx = 40
const barGapPx = 10
const cellPaddingPx = 28
const longestTextCh = 16

export function textWidthCh(texts: readonly string[]): number {
  return Math.min(
    longestTextCh,
    Math.max(1, ...texts.map((text) => text.length))
  )
}

export function barShare(value: number | null, max: number): number {
  if (value === null || !Number.isFinite(value) || max <= 0) return 0
  return Math.min(1, Math.max(0, value / max))
}

export function markerShare(value: number | null, max: number): number | null {
  if (value === null || max <= 0 || value > max) return null
  return barShare(value, max)
}

export function barColumnMinWidth(widthCh: number, chPx = 8): number {
  return Math.ceil(widthCh * chPx) + barGapPx + barMinWidthPx + cellPaddingPx
}
