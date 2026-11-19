import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { styleForSlot } from "@/panels/colors"
import {
  DragSelection,
  spanColumns,
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
    <div
      className={cn(
        "grid h-10 shrink-0 items-center border-b border-border",
        spanColumns,
        "narrow:grid-cols-[auto_minmax(0,1fr)_auto] narrow:grid-rows-none"
      )}
    >
      <span className="truncate px-2 text-xs text-muted-foreground">
        Whole trace, drag to zoom
      </span>
      <div className="min-w-0 px-2">
        <div
          role="img"
          aria-label="Overview of the whole trace"
          className="relative h-[34px] cursor-col-resize touch-none overflow-hidden select-none"
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
      </div>
      {zoomed ? (
        <Button
          type="button"
          variant="ghost"
          size="xs"
          className="justify-self-end"
          onClick={() => onZoom(wholeTrace)}
        >
          Reset zoom
        </Button>
      ) : null}
    </div>
  )
}
