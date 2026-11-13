import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { styleForSlot } from "@/panels/colors"
import {
  DragSelection,
  useZoomDrag,
  type TraceViewProps,
} from "@/panels/traces/traces-panel"

import { isWholeTrace, minimapRows, wholeTrace, type Zoom } from "./minimap"

const stripHeight = 34

export function TraceMinimap({
  model,
  zoom,
  onZoom,
}: {
  model: TraceViewProps["model"]
  zoom: Zoom
  onZoom: (zoom: Zoom) => void
}) {
  const rows = minimapRows(model.spans, stripHeight)
  const drag = useZoomDrag(zoom, onZoom)
  const zoomed = !isWholeTrace(zoom)
  return (
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-2">
      <span className="shrink-0 text-xs text-muted-foreground">
        Whole trace, drag to zoom
      </span>
      <div
        role="img"
        aria-label="Overview of the whole trace"
        className="relative h-[34px] min-w-0 flex-1 cursor-col-resize touch-none overflow-hidden select-none"
        {...drag.handlers}
      >
        {rows.map((row, index) => (
          <span
            key={index}
            aria-hidden="true"
            className={cn("absolute h-px", row.error && "bg-status-critical")}
            style={{
              top: row.top,
              left: `${row.left * 100}%`,
              width: `${row.width * 100}%`,
              minWidth: 1,
              backgroundColor: row.error
                ? undefined
                : styleForSlot(row.slot).color,
            }}
          />
        ))}
        {zoomed ? (
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-y-0 border-x border-ring bg-selection/40"
            style={{
              left: `${zoom.start * 100}%`,
              width: `${(zoom.end - zoom.start) * 100}%`,
            }}
          />
        ) : null}
        <DragSelection selection={drag.selection} />
      </div>
      {zoomed ? (
        <Button
          type="button"
          variant="ghost"
          size="xs"
          className="shrink-0"
          onClick={() => onZoom(wholeTrace)}
        >
          Reset zoom
        </Button>
      ) : null}
    </div>
  )
}
