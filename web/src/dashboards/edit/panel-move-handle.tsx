import { useId, type KeyboardEvent } from "react"
import { GripVerticalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { Panel } from "@/spec"

import { moveItem, resizeItem, stepItemVertically } from "./layout"
import { useLayoutStore } from "./layout-store"

const arrows: Record<string, { dx: number; dy: number }> = {
  ArrowLeft: { dx: -1, dy: 0 },
  ArrowRight: { dx: 1, dy: 0 },
  ArrowUp: { dx: 0, dy: -1 },
  ArrowDown: { dx: 0, dy: 1 },
}

export function PanelMoveHandle({
  panel,
}: {
  panel: Pick<Panel, "id" | "title">
}) {
  const hintId = useId()

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const arrow = arrows[event.key]
    if (!arrow || event.altKey || event.ctrlKey || event.metaKey) return
    event.preventDefault()
    const { session, setLayout } = useLayoutStore.getState()
    if (!session) return
    let next
    if (event.shiftKey) {
      next = resizeItem(session.layout, panel.id, arrow.dx, arrow.dy)
    } else if (arrow.dy !== 0) {
      next = stepItemVertically(session.layout, panel.id, arrow.dy < 0 ? -1 : 1)
    } else {
      next = moveItem(session.layout, panel.id, arrow.dx, 0)
    }
    setLayout(next, panel.id)
  }

  return (
    <>
      <Button
        variant="ghost"
        size="icon-xs"
        className="panel-drag-handle cursor-grab active:cursor-grabbing"
        aria-label={`Move or resize ${panel.title}`}
        aria-describedby={hintId}
        onKeyDown={onKeyDown}
      >
        <GripVerticalIcon />
      </Button>
      <span id={hintId} className="sr-only">
        Arrow keys move. Shift and arrow keys resize.
      </span>
    </>
  )
}

export function PanelResizeGrip() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 10 10"
      className="pointer-events-none absolute right-1 bottom-1 size-2.5 text-muted-foreground"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.25"
      strokeLinecap="round"
    >
      <path d="M9 3L3 9M9 6.5L6.5 9" />
    </svg>
  )
}

export function LayoutAnnouncer() {
  const message = useLayoutStore((state) => {
    const { session } = state
    if (!session?.movedPanelId) return ""
    const item = session.layout.find(
      (entry) => entry.i === session.movedPanelId
    )
    const panel = session.baseSpec.panels.find(
      (entry) => entry.id === session.movedPanelId
    )
    if (!item || !panel) return ""
    return `${panel.title}: column ${item.x + 1}, row ${item.y + 1}, ${item.w} by ${item.h}`
  })
  return (
    <div aria-live="polite" className="sr-only">
      {message}
    </div>
  )
}
