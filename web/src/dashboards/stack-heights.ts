import { gridMargin } from "./fit-layout"

export const gridRowHeight = 30

export function pixelHeight(h: number): number {
  return h * gridRowHeight + (h - 1) * gridMargin
}

export function stackedHeight(
  h: number,
  minHeight: number | undefined
): number {
  return Math.max(pixelHeight(h), minHeight ?? 0)
}

export function rowsFor(pixels: number): number {
  return Math.max(
    1,
    Math.ceil((pixels + gridMargin) / (gridRowHeight + gridMargin))
  )
}
