const barMinWidthPx = 40
const barGapPx = 10
const cellPaddingPx = 28

export function textWidthCh(texts: readonly string[]): number {
  return Math.max(1, ...texts.map((text) => text.length))
}

export function barShare(value: number | null, max: number): number {
  if (value === null || !Number.isFinite(value) || max <= 0) return 0
  return Math.min(1, Math.max(0, value / max))
}

export function barColumnMinWidth(widthCh: number, chPx = 8): number {
  return Math.ceil(widthCh * chPx) + barGapPx + barMinWidthPx + cellPaddingPx
}
