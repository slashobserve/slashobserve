import { useId } from "react"
import { GripVerticalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { liveRegionText } from "@/lib/live-region"

import { blockTitle, type DropTarget } from "./block-rows"
import type { Block } from "./queries"
import { handleId, type Arrange } from "./use-arrange"

const revealOnHover =
  "opacity-0 group-focus-within/block:opacity-100 group-hover/block:opacity-100 focus-visible:opacity-100"

export function ArrangeHandle({
  block,
  arrange,
}: {
  block: Block
  arrange: Arrange
}) {
  const hintId = useId()
  return (
    <>
      <Button
        id={handleId(block.id)}
        variant="ghost"
        size="icon-sm"
        aria-label={`Move or resize ${blockTitle(block)}`}
        aria-describedby={hintId}
        className={`absolute -top-6 left-0 z-10 cursor-grab touch-none bg-background active:cursor-grabbing ${revealOnHover}`}
        onPointerDown={arrange.startDrag(block)}
        onPointerMove={arrange.moveDrag}
        onPointerUp={arrange.endDrag}
        onPointerCancel={arrange.cancelDrag}
        onKeyDown={arrange.onKeyDown(block)}
      >
        <GripVerticalIcon aria-hidden="true" />
      </Button>
      <span id={hintId} className="sr-only">
        Arrow keys move the block. Shift and left or right arrow change its
        width. Shift and up or down arrow change its height.
      </span>
    </>
  )
}

export function ResizeGrip({
  block,
  arrange,
}: {
  block: Block
  arrange: Arrange
}) {
  return (
    <div
      aria-hidden="true"
      className={`absolute right-0 bottom-0 z-10 flex size-5 cursor-nwse-resize touch-none items-end justify-end p-1 text-muted-foreground ${revealOnHover}`}
      onPointerDown={arrange.startResize(block)}
      onPointerMove={arrange.moveResize}
      onPointerUp={arrange.endResize}
      onPointerCancel={arrange.cancelResize}
    >
      <svg
        viewBox="0 0 10 10"
        className="size-2.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.25"
        strokeLinecap="round"
      >
        <path d="M9 3L3 9M9 6.5L6.5 9" />
      </svg>
    </div>
  )
}

export function DropBar({ drop }: { drop: DropTarget }) {
  const position = drop.sideBySide
    ? drop.after
      ? "inset-y-0 -right-2.5 w-0.5"
      : "inset-y-0 -left-2.5 w-0.5"
    : drop.after
      ? "inset-x-0 -bottom-3.5 h-0.5"
      : "inset-x-0 -top-3.5 h-0.5"
  return (
    <span
      aria-hidden="true"
      className={`pointer-events-none absolute z-10 bg-primary ${position}`}
    />
  )
}

export function ArrangeAnnouncer({ arrange }: { arrange: Arrange }) {
  return (
    <div aria-live="polite" className="sr-only">
      {liveRegionText(arrange.announcement.message, arrange.announcement.count)}
    </div>
  )
}
