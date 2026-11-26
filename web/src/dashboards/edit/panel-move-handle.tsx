import { useId, type KeyboardEvent } from "react"
import { GripVerticalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { liveRegionText } from "@/lib/live-region"
import { cn } from "@/lib/utils"
import { panelTitle, type LayoutPanel } from "@/spec"

import {
  moveItemHorizontally,
  resizeItemPushing,
  sameLayout,
  stepItemVertically,
} from "./layout"
import { useLayoutStore } from "./layout-store"

export const panelDragCancel =
  "button:not(.panel-drag-handle):not([data-panel-title]), a, input, textarea, [role='menu'], [role='menuitem']"

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
      next = resizeItemPushing(
        session.layout,
        session.membership,
        panel.id,
        arrow.dx,
        arrow.dy
      )
    } else if (arrow.dy !== 0) {
      next = stepItemVertically(
        session.layout,
        session.membership,
        panel.id,
        arrow.dy < 0 ? -1 : 1
      )
    } else {
      next = moveItemHorizontally(
        session.layout,
        session.membership,
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

export function PanelMoveHint({ shown }: { shown: boolean }) {
  return (
    <span
      aria-hidden="true"
      data-panel-move-hint=""
      className={cn(
        "absolute inset-y-0 left-0 flex w-3 items-center justify-center pt-[9px] pb-1 text-muted-foreground opacity-0",
        shown &&
          "group-hover/header:opacity-60 motion-safe:transition-opacity motion-safe:duration-100 motion-safe:ease-out"
      )}
    >
      <GripVerticalIcon className="size-3" />
    </span>
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
