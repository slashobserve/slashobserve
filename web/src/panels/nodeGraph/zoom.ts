export type View = { zoom: number; panX: number; panY: number }
export type Anchor = { x: number; y: number }

export const initialView: View = { zoom: 1, panX: 0, panY: 0 }
export const minZoom = 0.25
export const maxZoom = 4
export const zoomStep = 1.25

const pixelsPerLine = 16
const pixelsPerPage = 400
const wheelSensitivity = 0.002
const center: Anchor = { x: 0, y: 0 }

export function zoomView(
  view: View,
  factor: number,
  anchor: Anchor = center
): View {
  const zoom = Math.min(maxZoom, Math.max(minZoom, view.zoom * factor))
  const ratio = zoom / view.zoom
  return {
    zoom,
    panX: anchor.x * (1 - ratio) + view.panX * ratio,
    panY: anchor.y * (1 - ratio) + view.panY * ratio,
  }
}

export function wheelFactor(deltaY: number, deltaMode: number): number {
  const pixels =
    deltaMode === 1
      ? deltaY * pixelsPerLine
      : deltaMode === 2
        ? deltaY * pixelsPerPage
        : deltaY
  return Math.exp(-pixels * wheelSensitivity)
}
