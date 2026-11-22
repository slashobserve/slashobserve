import { useRef, type KeyboardEvent, type PointerEvent } from "react"
import { MoveDiagonal2Icon } from "lucide-react"

import {
  draggedBoxSize,
  keyboardBoxSize,
  viewportRoom,
  type BoxSize,
} from "./canvas-box-size"

type Props = {
  measure: () => BoxSize
  onResize: (size: BoxSize) => void
  onSettle: (size: BoxSize) => void
  onReset: () => void
}

type Drag = { x: number; y: number; start: BoxSize; last: BoxSize }

export function BoxResizeGrip({ measure, onResize, onSettle, onReset }: Props) {
  const drag = useRef<Drag | null>(null)
  function onPointerDown(event: PointerEvent<HTMLButtonElement>) {
    if (event.button !== 0) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    const start = measure()
    drag.current = { x: event.clientX, y: event.clientY, start, last: start }
  }
  function onPointerMove(event: PointerEvent<HTMLButtonElement>) {
    const held = drag.current
    if (held === null) return
    held.last = draggedBoxSize(
      held.start,
      { x: event.clientX - held.x, y: event.clientY - held.y },
      viewportRoom()
    )
    onResize(held.last)
  }
  function release() {
    const held = drag.current
    if (held === null) return
    drag.current = null
    if (held.last !== held.start) onSettle(held.last)
  }
  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const next = keyboardBoxSize(
      measure(),
      event.key,
      event.shiftKey,
      viewportRoom()
    )
    if (next === null) return
    event.preventDefault()
    event.stopPropagation()
    onResize(next)
    onSettle(next)
  }
  return (
    <button
      type="button"
      data-box-grip=""
      aria-label="Resize this box"
      title="Drag to resize, or use the arrow keys. Double-click for the default size."
      className="absolute right-0 bottom-0 flex size-4 cursor-nwse-resize touch-none items-center justify-center rounded-control text-muted-foreground opacity-60 outline-hidden hover:opacity-100 focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring motion-safe:transition-opacity motion-safe:duration-100"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={release}
      onPointerCancel={release}
      onDoubleClick={onReset}
      onKeyDown={onKeyDown}
    >
      <MoveDiagonal2Icon aria-hidden="true" className="size-3" />
    </button>
  )
}
