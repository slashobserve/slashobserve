export type CanvasBox = "comment" | "ask"

export type BoxSize = { width: number; height: number }

export type BoxRoom = { width: number; height: number }

const boxDefaultWidth: Record<CanvasBox, number> = {
  comment: 320,
  ask: 384,
}

export const boxMinWidth = 280
export const boxMinHeight = 280
const viewportGap = 16
const keyStep = 16
const shiftKeyStep = 64

export const boxSizeKey = "slashobserve-canvas-box-size"

type Stored = Partial<Record<CanvasBox, BoxSize>>

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0
}

function sizeOf(value: unknown): BoxSize | null {
  if (typeof value !== "object" || value === null) return null
  const { width, height } = value as Record<string, unknown>
  return positive(width) && positive(height) ? { width, height } : null
}

export function parseStoredBoxSizes(raw: string | null): Stored {
  if (raw === null) return {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== "object" || parsed === null) return {}
    const boxes = parsed as Record<string, unknown>
    const comment = sizeOf(boxes.comment)
    const ask = sizeOf(boxes.ask)
    return {
      ...(comment === null ? {} : { comment }),
      ...(ask === null ? {} : { ask }),
    }
  } catch {
    return {}
  }
}

function readStored(): Stored {
  try {
    return parseStoredBoxSizes(localStorage.getItem(boxSizeKey))
  } catch {
    return {}
  }
}

export function storedBoxSize(box: CanvasBox): BoxSize | null {
  return readStored()[box] ?? null
}

export function storeBoxSize(box: CanvasBox, size: BoxSize | null): void {
  try {
    const next: Stored = { ...readStored() }
    if (size === null) delete next[box]
    else next[box] = size
    if (Object.keys(next).length === 0) localStorage.removeItem(boxSizeKey)
    else localStorage.setItem(boxSizeKey, JSON.stringify(next))
  } catch {
    return
  }
}

export function viewportRoom(): BoxRoom {
  return {
    width: window.innerWidth - viewportGap,
    height: window.innerHeight - viewportGap,
  }
}

function between(value: number, least: number, most: number): number {
  return Math.round(Math.max(least, Math.min(Math.max(least, most), value)))
}

export function clampBoxSize(size: BoxSize, room: BoxRoom): BoxSize {
  return {
    width: between(size.width, boxMinWidth, room.width),
    height: between(size.height, boxMinHeight, room.height),
  }
}

export function draggedBoxSize(
  start: BoxSize,
  moved: { x: number; y: number },
  room: BoxRoom
): BoxSize {
  return clampBoxSize(
    { width: start.width + moved.x, height: start.height + moved.y },
    room
  )
}

export function keyboardBoxSize(
  size: BoxSize,
  key: string,
  shiftKey: boolean,
  room: BoxRoom
): BoxSize | null {
  const step = shiftKey ? shiftKeyStep : keyStep
  switch (key) {
    case "ArrowLeft":
      return clampBoxSize({ ...size, width: size.width - step }, room)
    case "ArrowRight":
      return clampBoxSize({ ...size, width: size.width + step }, room)
    case "ArrowUp":
      return clampBoxSize({ ...size, height: size.height - step }, room)
    case "ArrowDown":
      return clampBoxSize({ ...size, height: size.height + step }, room)
    default:
      return null
  }
}

export function boxStyle(
  box: CanvasBox,
  size: BoxSize | null
): { width: string; height?: string } {
  const widest = `calc(100vw - ${viewportGap}px)`
  if (size === null) {
    return { width: `min(${boxDefaultWidth[box]}px, ${widest})` }
  }
  return {
    width: `min(${size.width}px, ${widest})`,
    height: `min(${size.height}px, var(--available-height, 100vh))`,
  }
}
