import { useId, type KeyboardEvent } from "react"
import { GripVerticalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { liveRegionText } from "@/lib/live-region"
import { panelTitle, type LayoutPanel } from "@/spec"

import {
  moveItemHorizontally,
  resizeItemPushing,
  sameLayout,
  stepItemVertically,
} from "./layout"
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
  panel: Pick<LayoutPanel, "id" | "title">
}) {
  const hintId = useId()

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const arrow = arrows[event.key]
    if (!arrow || event.altKey || event.ctrlKey || event.metaKey) return
    event.preventDefault()
    const { session, commitLayout } = useLayoutStore.getState()
    if (!session) return
    let next
    if (event.shiftKey) {
      next = resizeItemPushing(session.layout, panel.id, arrow.dx, arrow.dy)
    } else if (arrow.dy !== 0) {
      next = stepItemVertically(session.layout, panel.id, arrow.dy < 0 ? -1 : 1)
    } else {
      next = moveItemHorizontally(
        session.layout,
        panel.id,
        arrow.dx < 0 ? -1 : 1
      )
    }
    const item = next.find((entry) => entry.i === panel.id)
    if (!item) return
    if (sameLayout(next, session.layout)) {
      const verb = event.shiftKey ? "resize" : "move"
      commitLayout(
        session.layout,
        `${panelTitle(panel)}: can't ${verb} further`
      )
      return
    }
    commitLayout(
      next,
      `${panelTitle(panel)}: column ${item.x + 1}, row ${item.y + 1}, ${item.w} by ${item.h}`
    )
  }

  return (
    <>
      <Button
        variant="ghost"
        size="icon-xs"
        className="panel-drag-handle cursor-grab active:cursor-grabbing"
        aria-label={`Move or resize ${panelTitle(panel)}`}
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
  const message = useLayoutStore((state) =>
    liveRegionText(
      state.session?.announcement ?? "",
      state.session?.announcementCount ?? 0
    )
  )
  return (
    <div aria-live="polite" className="sr-only">
      {message}
    </div>
  )
}
