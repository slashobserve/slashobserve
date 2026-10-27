import { useEffect, useId, useMemo, useRef, useState } from "react"
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react"

import { formatDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"

import { styleForSlot } from "../colors"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import { traceModel, visibleSpans, windowRows, type Span } from "./trace-model"

const rowHeight = 28
const windowAfter = 200
const overscan = 10
const indentStep = 12
const rulerTicks = [0, 0.25, 0.5, 0.75, 1]
const ErrorIcon = thresholdIcons.critical

type TraceProps = {
  model: Extract<ReturnType<typeof traceModel>, { reason: null }>
  width: number
  height: number
}

function tickClass(tick: number): string {
  if (tick === 0) return "left-0"
  if (tick === 1) return "right-0"
  return "-translate-x-1/2"
}

function SpanRow({
  span,
  collapsed,
  open,
  detailsId,
  formatDuration,
  onToggle,
  onOpen,
}: {
  span: Span
  collapsed: boolean
  open: boolean
  detailsId: string
  formatDuration: (ms: number) => string
  onToggle: () => void
  onOpen: () => void
}) {
  const CollapseIcon = collapsed ? ChevronRightIcon : ChevronDownIcon
  const label = span.operation || span.id
  return (
    <div
      className={cn(
        "grid grid-cols-[minmax(12rem,40%)_minmax(0,1fr)] items-center text-xs",
        open && "bg-selection"
      )}
      style={{ height: rowHeight - 1 }}
    >
      <div
        className="flex min-w-0 items-center gap-1 pr-2"
        style={{ paddingLeft: 8 + span.depth * indentStep }}
      >
        {span.hasChildren ? (
          <button
            type="button"
            aria-expanded={!collapsed}
            aria-label={`${collapsed ? "Expand" : "Collapse"} ${label}`}
            onClick={onToggle}
            className="flex size-5 shrink-0 items-center justify-center rounded-control text-muted-foreground outline-none hover:bg-selection focus-visible:ring-2 focus-visible:ring-ring"
          >
            <CollapseIcon className="size-3" aria-hidden="true" />
          </button>
        ) : (
          <span className="size-5 shrink-0" aria-hidden="true" />
        )}
        <button
          type="button"
          aria-expanded={open}
          aria-controls={open ? detailsId : undefined}
          onClick={onOpen}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-control text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="min-w-0 flex-1 truncate">
            {span.service ? (
              <span className="text-muted-foreground">{span.service} </span>
            ) : null}
            <span className="text-foreground">{span.operation || span.id}</span>
          </span>
          {span.error ? (
            <span className="flex shrink-0 items-center gap-1 text-foreground">
              <ErrorIcon
                className="size-3 text-status-critical"
                aria-hidden="true"
              />
              Error
            </span>
          ) : null}
          <span className="shrink-0 text-muted-foreground tabular-nums">
            {formatDuration(span.duration)}
          </span>
        </button>
      </div>
      <div className="h-full min-w-0 px-2" onClick={onOpen}>
        <div className="relative h-full">
          <div
            aria-hidden="true"
            className={cn(
              "absolute top-1/2 h-3 -translate-y-1/2 rounded-sm",
              span.error && "bg-status-critical"
            )}
            style={{
              left: `${span.offset * 100}%`,
              width: `${span.width * 100}%`,
              minWidth: 2,
              backgroundColor: span.error
                ? undefined
                : styleForSlot(span.slot).color,
            }}
          />
        </div>
      </div>
    </div>
  )
}

function Trace({ model, width, height }: TraceProps) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const [openId, setOpenId] = useState<string | null>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [detailHeight, setDetailHeight] = useState(0)
  const detailRef = useRef<HTMLDListElement>(null)
  const detailsId = useId()

  const rows = useMemo(
    () => visibleSpans(model.spans, collapsed),
    [model.spans, collapsed]
  )
  const openIndex = rows.findIndex((span) => span.id === openId)
  const windowed = rows.length > windowAfter
  const range = windowed
    ? windowRows({
        count: rows.length,
        rowHeight,
        scrollTop,
        viewport: height,
        openIndex,
        detailHeight,
        overscan,
      })
    : { start: 0, end: rows.length, before: 0, after: 0 }
  const detailRendered = openIndex >= range.start && openIndex < range.end

  useEffect(() => {
    const node = detailRef.current
    if (!node) return
    const observer = new ResizeObserver(() =>
      setDetailHeight(node.getBoundingClientRect().height)
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [openId, detailRendered])

  const toggle = (id: string) =>
    setCollapsed((current) => {
      const next = new Set(current)
      if (!next.delete(id)) next.add(id)
      return next
    })

  return (
    <div className="flex flex-col" style={{ width, height }}>
      <div className="shrink-0 border-b border-border px-2 py-1.5">
        <p className="truncate text-sm font-medium text-foreground">
          {[model.rootService, model.rootOperation].filter(Boolean).join(" ")}
        </p>
        <p className="flex flex-wrap gap-x-3 text-xs text-muted-foreground">
          <span>
            Trace <span className="font-mono">{model.traceId || "-"}</span>
          </span>
          <time dateTime={new Date(model.start).toISOString()}>
            {formatDateTimeMs(model.start)}
          </time>
          <span className="tabular-nums">
            {model.formatDuration(model.duration)}
          </span>
        </p>
      </div>
      {model.notes.map((note) => (
        <p
          key={note}
          className="shrink-0 border-b border-border px-2 py-1.5 text-xs text-muted-foreground"
        >
          {note}
        </p>
      ))}
      <div
        className="min-h-0 flex-1 overflow-auto"
        onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
      >
        <div className="sticky top-0 z-10 grid grid-cols-[minmax(12rem,40%)_minmax(0,1fr)] border-b border-border bg-background text-xs text-muted-foreground">
          <span className="px-2 py-1">Service and operation</span>
          <div className="relative h-6 px-2" aria-hidden="true">
            <div className="relative h-full">
              {rulerTicks.map((tick) => (
                <span
                  key={tick}
                  className={cn(
                    "absolute top-1 whitespace-nowrap tabular-nums",
                    tickClass(tick)
                  )}
                  style={
                    tick === 0 || tick === 1
                      ? undefined
                      : { left: `${tick * 100}%` }
                  }
                >
                  {model.formatDuration(model.duration * tick)}
                </span>
              ))}
            </div>
          </div>
        </div>
        <ul
          role="list"
          aria-label="Spans"
          data-span-count={rows.length}
          className="m-0 list-none p-0"
          style={{ paddingTop: range.before, paddingBottom: range.after }}
        >
          {rows.slice(range.start, range.end).map((span) => {
            const open = span.id === openId
            return (
              <li key={span.id} className="border-b border-border">
                <SpanRow
                  span={span}
                  collapsed={collapsed.has(span.id)}
                  open={open}
                  detailsId={detailsId}
                  formatDuration={model.formatDuration}
                  onToggle={() => toggle(span.id)}
                  onOpen={() => setOpenId(open ? null : span.id)}
                />
                {open ? (
                  <dl
                    ref={detailRef}
                    id={detailsId}
                    className="m-0 grid grid-cols-[minmax(5rem,max-content)_minmax(0,1fr)] gap-x-3 gap-y-0.5 border-t border-border px-2 py-2 text-xs"
                  >
                    {span.attributes.length === 0 ? (
                      <dd className="col-span-2 text-muted-foreground">
                        No attributes.
                      </dd>
                    ) : (
                      span.attributes.map((attribute, index) => (
                        <div
                          key={`${index}:${attribute.key}`}
                          className="contents"
                        >
                          <dt className="text-muted-foreground">
                            {attribute.key}
                          </dt>
                          <dd className="m-0 font-mono [overflow-wrap:anywhere] whitespace-pre-wrap">
                            {attribute.text}
                          </dd>
                        </div>
                      ))
                    )}
                  </dl>
                ) : null}
              </li>
            )
          })}
        </ul>
      </div>
    </div>
  )
}

export const TracesPanel: PanelRenderer = ({
  frames,
  fields,
  width,
  height,
}) => {
  const model = useMemo(() => traceModel(frames, fields), [frames, fields])
  if (model.reason !== null) {
    return (
      <div
        className="flex items-center justify-center p-4 text-center text-xs text-muted-foreground"
        style={{ width, height }}
      >
        {model.reason}
      </div>
    )
  }
  return <Trace model={model} width={width} height={height} />
}

export default TracesPanel
