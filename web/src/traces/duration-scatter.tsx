import { useNavigate } from "@tanstack/react-router"
import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
} from "react"

import { Button } from "@/components/ui/button"
import { traceLink, traceUrl } from "@/lib/links"
import { timeLabel } from "@/lib/time-ticks"
import { cn } from "@/lib/utils"

import {
  durationBoundText,
  durationScale,
  durationTicks,
  durationTickText,
  inSelection,
  nudgeSelection,
  scatterPoints,
  selectionFromDrag,
  selectionText,
  selectionTimeText,
  windowTicks,
  type DurationScale,
  type Selection,
  type SelectionHandle,
  type TimeWindow,
} from "./scatter"
import { durationText, openTraceLabel, type TraceRow } from "./trace-rows"

const plotHeight = 160
const maxTimeTicks = 6
const dragStartPx = 4

type Point = { x: number; y: number }

type Drag = {
  anchor: Point
  current: Point
  left: number
  top: number
  width: number
  height: number
  captured: boolean
}

function plainLeftClick(event: MouseEvent): boolean {
  return (
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey
  )
}

function isControl(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest("a, button") !== null
}

function Legend({ children }: { children: ReactNode }) {
  return <span className="flex items-center gap-1.5">{children}</span>
}

function useBrush(
  window: TimeWindow,
  scale: DurationScale,
  selection: Selection | null,
  onSelectionChange: (next: Selection | null) => void
) {
  const [drag, setDrag] = useState<Drag | null>(null)
  const dragged = useRef(false)
  const dragging = drag !== null

  useEffect(() => {
    if (!dragging) return
    const cancel = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") setDrag(null)
    }
    globalThis.addEventListener("keydown", cancel)
    return () => globalThis.removeEventListener("keydown", cancel)
  }, [dragging])

  function release(event: PointerEvent<HTMLElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  const handlers = {
    onPointerDown(event: PointerEvent<HTMLElement>) {
      if (event.button !== 0) return
      dragged.current = false
      const rect = event.currentTarget.getBoundingClientRect()
      const anchor = {
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      }
      setDrag({
        anchor,
        current: anchor,
        left: rect.left,
        top: rect.top,
        width: rect.width,
        height: rect.height,
        captured: false,
      })
    },
    onPointerMove(event: PointerEvent<HTMLElement>) {
      if (drag === null) return
      const current = {
        x: event.clientX - drag.left,
        y: event.clientY - drag.top,
      }
      const far =
        Math.abs(current.x - drag.anchor.x) >= dragStartPx ||
        Math.abs(current.y - drag.anchor.y) >= dragStartPx
      if (!drag.captured && far) {
        event.currentTarget.setPointerCapture(event.pointerId)
      }
      setDrag({ ...drag, current, captured: drag.captured || far })
    },
    onPointerUp(event: PointerEvent<HTMLElement>) {
      release(event)
      if (drag?.captured) {
        dragged.current = true
        onSelectionChange(
          selectionFromDrag(drag.anchor, drag.current, drag, window, scale)
        )
      }
      setDrag(null)
    },
    onPointerCancel(event: PointerEvent<HTMLElement>) {
      release(event)
      setDrag(null)
    },
    onClickCapture(event: MouseEvent<HTMLElement>) {
      if (dragged.current) {
        dragged.current = false
        event.preventDefault()
        event.stopPropagation()
        return
      }
      if (selection !== null && !isControl(event.target)) {
        onSelectionChange(null)
      }
    },
  }

  const live =
    drag?.captured === true
      ? {
          left: Math.max(0, Math.min(drag.anchor.x, drag.current.x)),
          top: Math.max(0, Math.min(drag.anchor.y, drag.current.y)),
          right: Math.min(drag.width, Math.max(drag.anchor.x, drag.current.x)),
          bottom: Math.min(
            drag.height,
            Math.max(drag.anchor.y, drag.current.y)
          ),
        }
      : null
  return { handlers, live }
}

function SelectionHandleButton({
  handle,
  label,
  className,
  onNudge,
}: {
  handle: SelectionHandle
  label: string
  className: string
  onNudge: (handle: SelectionHandle, steps: number) => void
}) {
  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const horizontal = handle !== "floor"
    const steps =
      event.key === (horizontal ? "ArrowRight" : "ArrowUp")
        ? 1
        : event.key === (horizontal ? "ArrowLeft" : "ArrowDown")
          ? -1
          : 0
    if (steps === 0) return
    event.preventDefault()
    onNudge(handle, steps)
  }
  return (
    <button
      type="button"
      aria-label={label}
      onKeyDown={onKeyDown}
      className={cn(
        "pointer-events-auto absolute size-3 rounded-full border border-ring bg-background outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className
      )}
    />
  )
}

function SelectionBox({
  selection,
  window,
  scale,
  onSelectionChange,
}: {
  selection: Selection
  window: TimeWindow
  scale: DurationScale
  onSelectionChange: (next: Selection | null) => void
}) {
  const xSpan = window.end - window.start
  const ceiling = selection.maxMs === null ? 1 : scale.y(selection.maxMs)
  const floor = scale.y(selection.minMs)
  const percent = (fraction: number) => `${(fraction * 100).toFixed(2)}%`
  const nudge = (handle: SelectionHandle, steps: number) =>
    onSelectionChange(nudgeSelection(selection, handle, steps, window, scale))
  return (
    <div
      role="group"
      aria-label="Selection on the chart"
      onPointerDown={(event) => event.stopPropagation()}
      className="absolute border border-ring bg-selection/40"
      style={{
        left: percent((selection.from - window.start) / xSpan),
        width: percent((selection.to - selection.from) / xSpan),
        top: percent(1 - ceiling),
        height: percent(ceiling - floor),
      }}
    >
      <SelectionHandleButton
        handle="start"
        label={`Move the selection start, now ${selectionTimeText(selection.from, undefined)}`}
        className="top-1/2 left-0 -translate-x-1/2 -translate-y-1/2 cursor-ew-resize"
        onNudge={nudge}
      />
      <SelectionHandleButton
        handle="end"
        label={`Move the selection end, now ${selectionTimeText(selection.to, undefined)}`}
        className="top-1/2 right-0 translate-x-1/2 -translate-y-1/2 cursor-ew-resize"
        onNudge={nudge}
      />
      <SelectionHandleButton
        handle="floor"
        label={`Move the duration floor, now ${durationBoundText(selection.minMs)}`}
        className="bottom-0 left-1/2 -translate-x-1/2 translate-y-1/2 cursor-ns-resize"
        onNudge={nudge}
      />
    </div>
  )
}

export function DurationScatter({
  rows,
  range,
  window,
  datasourceUid,
  highlighted,
  onHighlight,
  p95Ms,
  p95Label,
  ariaLabel = "Duration of matching traces",
  overlay,
  selection,
  onSelectionChange,
  selectedCount,
  selectionActions,
}: {
  rows: readonly TraceRow[]
  range: { from: string; to: string }
  window: TimeWindow
  datasourceUid: string
  highlighted: string | null
  onHighlight: (traceId: string | null) => void
  p95Ms: number | null
  p95Label: string
  ariaLabel?: string
  overlay?: ReactNode
  selection: Selection | null
  onSelectionChange: (next: Selection | null) => void
  selectedCount: number
  selectionActions?: ReactNode
}) {
  const navigate = useNavigate()
  const slowest = Math.max(0, ...rows.map((row) => row.durationMs ?? 0))
  const scale = durationScale(slowest)
  const points = scatterPoints(rows, window, slowest)
  const byId = new Map(rows.map((row) => [row.traceId, row]))
  const yTicks = durationTicks(slowest)
  const xTicks = windowTicks(window, maxTimeTicks)
  const xSpan = window.end - window.start
  const percent = (fraction: number) => `${(fraction * 100).toFixed(2)}%`
  const brush = useBrush(window, scale, selection, onSelectionChange)

  function pointOpacity(row: TraceRow, active: boolean): string {
    if (active) return "opacity-100"
    if (selection === null) return "opacity-70 hover:opacity-100"
    return inSelection(row, selection) ? "opacity-100" : "opacity-30"
  }

  return (
    <section
      aria-label="Duration over time"
      className="flex flex-col gap-1.5 rounded-card border bg-background px-3.5 pt-3 pb-2.5 narrow:hidden"
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1.5">
        <div className="flex flex-wrap items-center gap-2.5">
          <h2 className="text-[13px] font-medium">Duration over time</h2>
          {selection === null ? (
            <p className="text-xs text-muted-foreground">
              Each dot is a trace. Drag across the chart to select some and see
              what sets them apart.
            </p>
          ) : (
            <>
              <p className="text-xs text-muted-foreground">
                {selectionText(selection, selectedCount, undefined)}
              </p>
              <Button
                type="button"
                variant="ghost"
                size="xs"
                aria-label="Clear the selection"
                onClick={() => onSelectionChange(null)}
              >
                Clear
              </Button>
              {selectionActions}
            </>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-x-3.5 gap-y-1 text-xs">
          <Legend>
            <span
              aria-hidden="true"
              className="size-2 rounded-full bg-series-1"
            />
            OK
          </Legend>
          <Legend>
            <span
              aria-hidden="true"
              className="size-2 rounded-full bg-status-critical"
            />
            Error
          </Legend>
          {p95Ms !== null ? (
            <Legend>
              <span
                aria-hidden="true"
                className="w-3.5 border-t border-dashed border-series-3"
              />
              {p95Label}
            </Legend>
          ) : null}
        </div>
      </div>
      <div className="grid grid-cols-[44px_minmax(0,1fr)] gap-x-1.5">
        <div
          aria-hidden="true"
          className="relative text-[10px] text-muted-foreground tabular-nums"
          style={{ height: plotHeight }}
        >
          {yTicks.map((tick) => (
            <span
              key={tick}
              className="absolute right-0 -translate-y-1/2"
              style={{ top: percent(1 - scale.y(tick)) }}
            >
              {durationTickText(tick)}
            </span>
          ))}
        </div>
        <div
          className="relative cursor-crosshair touch-none select-none"
          style={{ height: plotHeight }}
          {...brush.handlers}
        >
          <svg
            role="img"
            aria-label={
              selection === null
                ? ariaLabel
                : `${ariaLabel}; ${selectedCount} of them are selected`
            }
            className="block size-full overflow-visible"
            onPointerLeave={() => onHighlight(null)}
          >
            {yTicks.map((tick) => (
              <line
                key={tick}
                x1="0"
                x2="100%"
                y1={percent(1 - scale.y(tick))}
                y2={percent(1 - scale.y(tick))}
                className="stroke-border"
              />
            ))}
            {p95Ms !== null ? (
              <line
                x1="0"
                x2="100%"
                y1={percent(1 - scale.y(p95Ms))}
                y2={percent(1 - scale.y(p95Ms))}
                stroke="var(--series-3)"
                strokeDasharray="4 4"
                vectorEffect="non-scaling-stroke"
              />
            ) : null}
            {points.map((point) => {
              const row = byId.get(point.traceId)
              if (!row) return null
              const active = highlighted === point.traceId
              return (
                <a
                  key={point.traceId}
                  href={traceUrl(row.traceId, datasourceUid, range)}
                  aria-label={openTraceLabel(row)}
                  onClick={(event) => {
                    if (!plainLeftClick(event)) return
                    event.preventDefault()
                    void navigate(traceLink(row.traceId, datasourceUid, range))
                  }}
                  onPointerEnter={() => onHighlight(point.traceId)}
                  onFocus={() => onHighlight(point.traceId)}
                  onBlur={() => onHighlight(null)}
                  className="outline-none focus-visible:[&_circle]:stroke-ring"
                >
                  <title>
                    {[row.service, row.span, durationText(row.durationMs)]
                      .filter((part) => part !== "")
                      .join(" · ")}
                  </title>
                  <circle
                    cx={percent(point.x)}
                    cy={percent(1 - point.y)}
                    r={active ? 4.5 : 3}
                    fill={
                      point.error ? "var(--status-critical)" : "var(--series-1)"
                    }
                    strokeWidth="2"
                    className={pointOpacity(row, active)}
                  />
                </a>
              )
            })}
          </svg>
          <div className="pointer-events-none absolute inset-0">
            {brush.live !== null ? (
              <div
                aria-hidden="true"
                className="absolute border border-ring bg-selection/40"
                style={{
                  left: brush.live.left,
                  top: brush.live.top,
                  width: brush.live.right - brush.live.left,
                  height: brush.live.bottom - brush.live.top,
                }}
              />
            ) : selection !== null ? (
              <SelectionBox
                selection={selection}
                window={window}
                scale={scale}
                onSelectionChange={onSelectionChange}
              />
            ) : null}
            {overlay}
          </div>
        </div>
      </div>
      <div
        aria-hidden="true"
        className="relative ml-[50px] h-4 text-[10px] text-muted-foreground tabular-nums"
      >
        {xTicks.map((tick) => (
          <span
            key={tick}
            className="absolute -translate-x-1/2"
            style={{ left: percent((tick - window.start) / xSpan) }}
          >
            {timeLabel(tick, xSpan, undefined)}
          </span>
        ))}
      </div>
    </section>
  )
}
