import { useEffect, useId, useMemo, useRef, useState, type Ref } from "react"
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react"

import { zonedDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"

import { styleForSlot } from "../colors"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import { niceTicks } from "./ruler"
import { filterSpans, spanFilters, type SpanFilters } from "./span-filters"
import { traceModel, visibleSpans, windowRows, type Span } from "./trace-model"

const rowHeight = 28
const windowAfter = 200
const overscan = 10
const indentStep = 12
const ErrorIcon = thresholdIcons.critical

export type TraceViewProps = {
  model: Extract<ReturnType<typeof traceModel>, { reason: null }>
  filters: SpanFilters
  zone: string | undefined
  width: number
  height: number
  attributes: "inline" | "pane"
}

function tickClass(tick: number, duration: number): string {
  if (tick === 0) return "left-0"
  if (tick >= duration) return "right-0"
  return "-translate-x-1/2"
}

function SpanRow({
  span,
  collapsed,
  open,
  matched,
  detailsId,
  formatDuration,
  onToggle,
  onOpen,
}: {
  span: Span
  collapsed: boolean
  open: boolean
  matched: boolean | null
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
            <span
              className={
                matched === false ? "text-muted-foreground" : "text-foreground"
              }
            >
              {span.operation || span.id}
            </span>
            {matched ? (
              <span className="sr-only"> Matches the filters</span>
            ) : null}
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
              span.error && "bg-status-critical",
              matched === false && "opacity-30"
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

function SpanAttributes({
  ref,
  id,
  span,
  className,
}: {
  ref?: Ref<HTMLDListElement>
  id?: string
  span: Span
  className: string
}) {
  return (
    <dl
      ref={ref}
      id={id}
      className={cn(
        "m-0 grid grid-cols-[minmax(5rem,max-content)_minmax(0,1fr)] gap-x-3 gap-y-0.5 text-xs",
        className
      )}
    >
      {span.attributes.length === 0 ? (
        <dd className="col-span-2 text-muted-foreground">No attributes.</dd>
      ) : (
        span.attributes.map((attribute, index) => (
          <div key={`${index}:${attribute.key}`} className="contents">
            <dt className="text-muted-foreground">{attribute.key}</dt>
            <dd className="m-0 font-mono [overflow-wrap:anywhere] whitespace-pre-wrap">
              {attribute.text}
            </dd>
          </div>
        ))
      )}
    </dl>
  )
}

function SpanPane({
  id,
  span,
  zone,
  formatDuration,
}: {
  id: string
  span: Span | undefined
  zone: string | undefined
  formatDuration: (ms: number) => string
}) {
  return (
    <aside
      aria-label="Span attributes"
      id={id}
      className="min-w-56 shrink-0 basis-2/5 overflow-auto border-l border-border p-2 text-xs"
    >
      {span ? (
        <>
          <p className="truncate text-sm font-medium text-foreground">
            {[span.service, span.operation].filter(Boolean).join(" ")}
          </p>
          <p className="flex flex-wrap gap-x-3 text-muted-foreground">
            <span>
              Span <span className="font-mono">{span.id}</span>
            </span>
            <time dateTime={new Date(span.start).toISOString()}>
              {zonedDateTimeMs(span.start, zone)}
            </time>
            <span className="tabular-nums">
              {formatDuration(span.duration)}
            </span>
            {span.error ? (
              <span className="flex items-center gap-1 text-foreground">
                <ErrorIcon
                  className="size-3 text-status-critical"
                  aria-hidden="true"
                />
                Error
              </span>
            ) : null}
          </p>
          <SpanAttributes span={span} className="mt-2" />
        </>
      ) : (
        <p className="text-muted-foreground">
          Select a span to see its attributes.
        </p>
      )}
    </aside>
  )
}

export function TraceView({
  model,
  filters,
  zone,
  width,
  height,
  attributes,
}: TraceViewProps) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const [openId, setOpenId] = useState<string | null>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [detailHeight, setDetailHeight] = useState(0)
  const detailRef = useRef<HTMLDListElement>(null)
  const detailsId = useId()

  const filtered = useMemo(
    () => filterSpans(model.spans, filters),
    [model.spans, filters]
  )
  const rows = useMemo(
    () => visibleSpans(filtered.spans, collapsed),
    [filtered.spans, collapsed]
  )
  const ticks = niceTicks(model.duration)
  const openIndex = rows.findIndex((span) => span.id === openId)
  const inline = attributes === "inline"
  const windowed = rows.length > windowAfter
  const range = windowed
    ? windowRows({
        count: rows.length,
        rowHeight,
        scrollTop,
        viewport: height,
        openIndex: inline ? openIndex : -1,
        detailHeight: inline ? detailHeight : 0,
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
            {zonedDateTimeMs(model.start, zone)}
          </time>
          <span className="tabular-nums">
            {model.formatDuration(model.duration)}
          </span>
        </p>
      </div>
      {[...model.notes, ...filtered.notes].map((note) => (
        <p
          key={note}
          className="shrink-0 border-b border-border px-2 py-1.5 text-xs text-muted-foreground"
        >
          {note}
        </p>
      ))}
      <div className="flex min-h-0 flex-1">
        <div
          className="min-h-0 min-w-0 flex-1 overflow-auto"
          onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
        >
          <div className="sticky top-0 z-10 grid grid-cols-[minmax(12rem,40%)_minmax(0,1fr)] border-b border-border bg-background text-xs text-muted-foreground">
            <span className="px-2 py-1">Service and operation</span>
            <div className="relative h-6 px-2" aria-hidden="true">
              <div className="relative h-full">
                {ticks.map((tick) => (
                  <span
                    key={tick}
                    className={cn(
                      "absolute top-1 whitespace-nowrap tabular-nums",
                      tickClass(tick, model.duration)
                    )}
                    style={
                      tick === 0 || tick >= model.duration
                        ? undefined
                        : { left: `${(tick / model.duration) * 100}%` }
                    }
                  >
                    {model.formatDuration(tick)}
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
                    matched={
                      filtered.matched === null
                        ? null
                        : filtered.matched.has(span.id)
                    }
                    detailsId={detailsId}
                    formatDuration={model.formatDuration}
                    onToggle={() => toggle(span.id)}
                    onOpen={() => setOpenId(open ? null : span.id)}
                  />
                  {open && attributes === "inline" ? (
                    <SpanAttributes
                      ref={detailRef}
                      id={detailsId}
                      span={span}
                      className="border-t border-border px-2 py-2"
                    />
                  ) : null}
                </li>
              )
            })}
          </ul>
        </div>
        {inline ? null : (
          <SpanPane
            id={detailsId}
            span={openIndex === -1 ? undefined : rows[openIndex]}
            zone={zone}
            formatDuration={model.formatDuration}
          />
        )}
      </div>
    </div>
  )
}

const TracesPanel: PanelRenderer = ({
  frames,
  fields,
  display,
  zone,
  width,
  height,
}) => {
  const model = useMemo(() => traceModel(frames, fields), [frames, fields])
  const filters = useMemo(() => spanFilters(display), [display])
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
  return (
    <TraceView
      model={model}
      filters={filters}
      zone={zone}
      width={width}
      height={height}
      attributes="inline"
    />
  )
}

export default TracesPanel
