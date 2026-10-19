import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from "react"

import { clampWidth, maxWidthFor, type WidthBounds } from "./card-width"

const keyStep = 16
const bigKeyStep = 64

type Props = {
  width: number
  bounds: WidthBounds
  onChange: (width: number) => void
}

export function ResizeHandle({ width, bounds, onChange }: Props) {
  const [dragging, setDragging] = useState(false)
  const start = useRef({ x: 0, width: 0 })

  useEffect(() => {
    if (!dragging) return
    const previous = document.body.style.userSelect
    document.body.style.userSelect = "none"
    return () => {
      document.body.style.userSelect = previous
    }
  }, [dragging])

  function change(next: number) {
    onChange(clampWidth(next, bounds, window.innerWidth))
  }

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    start.current = { x: event.clientX, width }
    setDragging(true)
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    if (!dragging) return
    change(start.current.width + (start.current.x - event.clientX))
  }

  function endDrag(event: PointerEvent<HTMLDivElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    setDragging(false)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const step = event.shiftKey ? bigKeyStep : keyStep
    switch (event.key) {
      case "ArrowLeft":
        change(width + step)
        break
      case "ArrowRight":
        change(width - step)
        break
      case "Home":
        change(bounds.min)
        break
      case "End":
        change(bounds.max)
        break
      case "Enter":
        change(bounds.initial)
        break
      default:
        return
    }
    event.preventDefault()
  }

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize panel"
      aria-valuenow={width}
      aria-valuemin={bounds.min}
      aria-valuemax={maxWidthFor(bounds, window.innerWidth)}
      tabIndex={0}
      title="Drag to resize, double-click to reset"
      data-dragging={dragging ? "" : undefined}
      className="group absolute inset-y-0 -left-[9px] z-10 grid w-2.5 cursor-col-resize touch-none place-items-center outline-none"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onDoubleClick={() => change(bounds.initial)}
      onKeyDown={onKeyDown}
    >
      <span
        aria-hidden="true"
        className="h-9 w-1 rounded-sm bg-border group-hover:bg-ring group-focus-visible:bg-ring group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-ring group-data-[dragging]:bg-ring"
      />
    </div>
  )
}
