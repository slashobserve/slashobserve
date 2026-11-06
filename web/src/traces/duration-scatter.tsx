import { useNavigate } from "@tanstack/react-router"
import type { MouseEvent, ReactNode } from "react"

import { traceLink, traceUrl } from "@/lib/links"
import { timeLabel } from "@/lib/time-ticks"
import { cn } from "@/lib/utils"

import {
  durationScale,
  durationTicks,
  durationTickText,
  scatterPoints,
  windowTicks,
  type TimeWindow,
} from "./scatter"
import { durationText, openTraceLabel, type TraceRow } from "./trace-rows"

const plotHeight = 160
const maxTimeTicks = 6

function plainLeftClick(event: MouseEvent): boolean {
  return (
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey
  )
}

function Legend({ children }: { children: ReactNode }) {
  return <span className="flex items-center gap-1.5">{children}</span>
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

  return (
    <section
      aria-label="Duration over time"
      className="flex flex-col gap-1.5 rounded-card border bg-background px-3.5 pt-3 pb-2.5 narrow:hidden"
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1.5">
        <div className="flex items-center gap-2.5">
          <h2 className="text-[13px] font-medium">Duration over time</h2>
          <p className="text-xs text-muted-foreground">Each dot is a trace.</p>
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
        <div className="relative" style={{ height: plotHeight }}>
          <svg
            role="img"
            aria-label={ariaLabel}
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
                    className={cn(
                      active ? "opacity-100" : "opacity-70 hover:opacity-100"
                    )}
                  />
                </a>
              )
            })}
          </svg>
          {overlay !== undefined ? (
            <div className="absolute inset-0">{overlay}</div>
          ) : null}
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
