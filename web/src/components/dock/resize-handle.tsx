import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from "react"

import { cn } from "@/lib/utils"

import {
  clampWidth,
  draggedWidth,
  maxWidthFor,
  storeWidth,
  type HandleSide,
  type WidthBounds,
} from "./card-width"

const keyStep = 16
const bigKeyStep = 64

type Props = {
  width: number
  bounds: WidthBounds
  onChange: (width: number) => void
  storageKey?: string
  side?: HandleSide
  label?: string
  fitViewport?: boolean
  className?: string | undefined
}

export function ResizeHandle({
  width,
  bounds,
  onChange,
  storageKey,
  side = "left",
  label = "Resize panel",
  fitViewport = true,
  className,
}: Props) {
  const [dragging, setDragging] = useState(false)
  const start = useRef({ x: 0, width: 0 })
  const latest = useRef(width)
  const viewport = () =>
    fitViewport ? window.innerWidth : Number.POSITIVE_INFINITY

  useEffect(() => {
    if (!dragging) return
    const previous = document.body.style.userSelect
    document.body.style.userSelect = "none"
    return () => {
      document.body.style.userSelect = previous
    }
  }, [dragging])

  function change(next: number) {
    latest.current = clampWidth(next, bounds, viewport())
    onChange(latest.current)
    return latest.current
  }

  function commit(next: number) {
    if (storageKey !== undefined) storeWidth(storageKey, next)
  }

  function changeAndCommit(next: number) {
    commit(change(next))
  }

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    start.current = { x: event.clientX, width }
    latest.current = width
    setDragging(true)
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    if (!dragging) return
    change(
      draggedWidth(side, start.current.width, start.current.x, event.clientX)
    )
  }

  function endDrag(event: PointerEvent<HTMLDivElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    if (!dragging) return
    setDragging(false)
    commit(latest.current)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const step = event.shiftKey ? bigKeyStep : keyStep
    const widen = side === "left" ? "ArrowLeft" : "ArrowRight"
    const narrow = side === "left" ? "ArrowRight" : "ArrowLeft"
    switch (event.key) {
      case widen:
        changeAndCommit(width + step)
        break
      case narrow:
        changeAndCommit(width - step)
        break
      case "Home":
        changeAndCommit(bounds.min)
        break
      case "End":
        changeAndCommit(bounds.max)
        break
      case "Enter":
        changeAndCommit(bounds.initial)
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
      aria-label={label}
      aria-valuenow={width}
      aria-valuemin={bounds.min}
      aria-valuemax={maxWidthFor(bounds, viewport())}
      tabIndex={0}
      title="Drag to resize, double-click to reset"
      data-dragging={dragging ? "" : undefined}
      className={cn(
        "group absolute inset-y-0 z-10 grid w-2.5 cursor-col-resize touch-none place-items-center outline-none",
        side === "left" ? "-left-[9px]" : "-right-[9px]",
        className
      )}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onDoubleClick={() => changeAndCommit(bounds.initial)}
      onKeyDown={onKeyDown}
    >
      <span
        aria-hidden="true"
        className="h-9 w-1 rounded-sm bg-border group-hover:bg-ring group-focus-visible:bg-ring group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-ring group-data-[dragging]:bg-ring"
      />
    </div>
  )
}
