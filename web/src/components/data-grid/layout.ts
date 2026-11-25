const virtualiseAbove = 100
export const minGridHeight = 384
const pageBottomGap = 48

export type GridLayout = "autoHeight" | "normal"

export function gridLayout(rowCount: number): GridLayout {
  return rowCount > virtualiseAbove ? "normal" : "autoHeight"
}

export function fillHeight(visibleHeight: number, top: number): number {
  return Math.max(
    minGridHeight,
    Math.floor(visibleHeight - Math.max(0, top) - pageBottomGap)
  )
}
