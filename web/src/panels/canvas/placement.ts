import { zoomView, type View } from "../nodeGraph/zoom"

export type Box = { left: number; top: number; width: number; height: number }
export type Size = { width: number; height: number }

type AxisMode = "start" | "end" | "both" | "center" | "scale"

type Axis = {
  mode: AxisMode
  start: number | null
  end: number | null
  size: number | null
}

export type Placement = { x: Axis; y: Axis }

const horizontalModes: Record<string, AxisMode> = {
  left: "start",
  right: "end",
  leftright: "both",
  center: "center",
  scale: "scale",
}

const verticalModes: Record<string, AxisMode> = {
  top: "start",
  bottom: "end",
  topbottom: "both",
  center: "center",
  scale: "scale",
}

function numberOrNull(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

function modeOf(
  table: Record<string, AxisMode>,
  value: unknown
): AxisMode | null {
  return typeof value === "string" && Object.hasOwn(table, value)
    ? table[value]
    : null
}

export function parsePlacement(raw: unknown): Placement | null {
  if (typeof raw !== "object" || raw === null || Array.isArray(raw)) {
    return null
  }
  const fields = raw as Record<string, unknown>
  const x = modeOf(horizontalModes, fields.horizontal)
  const y = modeOf(verticalModes, fields.vertical)
  if (x === null || y === null) return null
  return {
    x: {
      mode: x,
      start: numberOrNull(fields.left),
      end: numberOrNull(fields.right),
      size: numberOrNull(fields.width),
    },
    y: {
      mode: y,
      start: numberOrNull(fields.top),
      end: numberOrNull(fields.bottom),
      size: numberOrNull(fields.height),
    },
  }
}

function axisSpan(
  axis: Axis,
  extent: number,
  percent: { start: number; size: number }
): [number, number] {
  const fallback = {
    start: (percent.start * extent) / 100,
    size: (percent.size * extent) / 100,
  }
  const fallbackEnd = extent - fallback.start - fallback.size
  switch (axis.mode) {
    case "start":
      return [axis.start ?? fallback.start, axis.size ?? fallback.size]
    case "end": {
      const size = axis.size ?? fallback.size
      return [extent - (axis.end ?? fallbackEnd) - size, size]
    }
    case "both": {
      const start = axis.start ?? fallback.start
      return [start, extent - start - (axis.end ?? fallbackEnd)]
    }
    case "center": {
      const size = axis.size ?? fallback.size
      const offset =
        axis.start ?? fallback.start + fallback.size / 2 - extent / 2
      return [extent / 2 + offset - size / 2, size]
    }
    case "scale":
      return [
        ((axis.start ?? percent.start) * extent) / 100,
        ((axis.size ?? percent.size) * extent) / 100,
      ]
  }
}

const unplaced: Placement = {
  x: { mode: "scale", start: null, end: null, size: null },
  y: { mode: "scale", start: null, end: null, size: null },
}

export function elementBox(
  percent: Box,
  placement: Placement | null,
  body: Size
): Box {
  const place = placement ?? unplaced
  const [left, width] = axisSpan(place.x, body.width, {
    start: percent.left,
    size: percent.width,
  })
  const [top, height] = axisSpan(place.y, body.height, {
    start: percent.top,
    size: percent.height,
  })
  return { left, top, width: Math.max(0, width), height: Math.max(0, height) }
}

export function clampView(view: View, body: Size, infinite: boolean): View {
  if (infinite) return view
  const room = (extent: number) => Math.max(0, (extent * (view.zoom - 1)) / 2)
  const within = (value: number, limit: number) =>
    Math.min(limit, Math.max(-limit, value))
  return {
    zoom: view.zoom,
    panX: within(view.panX, room(body.width)),
    panY: within(view.panY, room(body.height)),
  }
}

export function zoomedView(
  view: View,
  factor: number,
  body: Size,
  infinite: boolean
): View {
  return clampView(zoomView(view, factor), body, infinite)
}
