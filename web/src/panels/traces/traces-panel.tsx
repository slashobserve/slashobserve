import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  type Ref,
} from "react"
import { ChevronDownIcon, ChevronRightIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { zonedDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"
import {
  wholeTrace,
  zoomedOffset,
  zoomFromDrag,
  type Zoom,
} from "@/traces/minimap"
import { highlightText } from "@/traces/trace-toolbar"

import { styleForSlot } from "../colors"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import { niceTicks } from "./ruler"
import {
  ancestorIds,
  filterSpans,
  spanFilters,
  type SpanFilters,
} from "./span-filters"
import { traceModel, visibleSpans, windowRows, type Span } from "./trace-model"

const rowHeight = 30
const windowAfter = 200
const overscan = 10
const indentStep = 12
const wideNameColumn = 600
const ErrorIcon = thresholdIcons.critical

export type TraceViewProps = {
  model: Extract<ReturnType<typeof traceModel>, { reason: null }>
  filters: SpanFilters
  zone: string | undefined
  width: number
  height: number
  attributes: "inline" | "pane"
  openId?: string | null
  onOpenIdChange?: (id: string | null) => void
  collapsed?: ReadonlySet<string>
  onCollapsedChange?: (collapsed: ReadonlySet<string>) => void
  toolbar?: ReactNode
  highlighted?: ReadonlySet<string> | null
  criticalPathIds?: ReadonlySet<string>
  dimmed?: ReadonlySet<string>
  zoom?: Zoom
  onZoom?: (zoom: Zoom) => void
}

type Drag = { anchor: number; x: number; left: number; width: number }

export function useZoomDrag(
  zoom: Zoom,
  onZoom: ((zoom: Zoom) => void) | undefined
) {
  const [drag, setDrag] = useState<Drag | null>(null)
  const dragging = drag !== null

  useEffect(() => {
    if (!dragging) return
    const cancel = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") setDrag(null)
    }
    window.addEventListener("keydown", cancel)
    return () => window.removeEventListener("keydown", cancel)
  }, [dragging])

  function release(event: PointerEvent<HTMLElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  const handlers = onZoom
    ? {
        onPointerDown(event: PointerEvent<HTMLElement>) {
          if (event.button !== 0) return
          const rect = event.currentTarget.getBoundingClientRect()
          event.currentTarget.setPointerCapture(event.pointerId)
          const x = event.clientX - rect.left
          setDrag({ anchor: x, x, left: rect.left, width: rect.width })
        },
        onPointerMove(event: PointerEvent<HTMLElement>) {
          const x = event.clientX
          setDrag((current) =>
            current ? { ...current, x: x - current.left } : current
          )
        },
        onPointerUp(event: PointerEvent<HTMLElement>) {
          release(event)
          if (drag) {
            const next = zoomFromDrag(drag.anchor, drag.x, drag.width, zoom)
            if (next) onZoom(next)
          }
          setDrag(null)
        },
        onPointerCancel(event: PointerEvent<HTMLElement>) {
          release(event)
          setDrag(null)
        },
      }
    : {}
  const selection =
    drag && drag.width > 0
      ? {
          left: Math.min(drag.anchor, drag.x) / drag.width,
          width: Math.abs(drag.x - drag.anchor) / drag.width,
        }
      : null
  return { handlers, selection }
}

export function DragSelection({
  selection,
}: {
  selection: { left: number; width: number } | null
}) {
  if (!selection) return null
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none absolute inset-y-0 border-x border-ring bg-selection/40"
      style={{
        left: `${selection.left * 100}%`,
        width: `${selection.width * 100}%`,
      }}
    />
  )
}

function tickClass(tick: number, duration: number): string {
  if (tick === 0) return "left-0"
  if (tick >= duration) return "right-0"
  return "-translate-x-1/2"
}

function nameColumns(width: number): string {
  return width < wideNameColumn
    ? "grid-cols-[minmax(12rem,40%)_minmax(0,1fr)]"
    : "grid-cols-[300px_minmax(0,1fr)]"
}

function SpanRow({
  span,
  columns,
  collapsed,
  open,
  matched,
  needle,
  onPath,
  dim,
  zoom,
  detailsId,
  formatDuration,
  onToggle,
  onOpen,
}: {
  span: Span
  columns: string
  collapsed: boolean
  open: boolean
  matched: boolean | null
  needle: string
  onPath: boolean | null
  dim: boolean
  zoom: Zoom
  detailsId: string
  formatDuration: (ms: number) => string
  onToggle: () => void
  onOpen: () => void
}) {
  const CollapseIcon = collapsed ? ChevronRightIcon : ChevronDownIcon
  const label = span.operation || span.id
  const bar = zoomedOffset(span, zoom)
  const highlight = matched && needle !== ""
  return (
    <div
      className={cn(
        "grid min-h-[30px] items-center text-xs",
        columns,
        open && "bg-selection",
        (dim || onPath === false) && "opacity-[.42]"
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
            tabIndex={-1}
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
          tabIndex={-1}
          aria-expanded={open}
          aria-controls={open ? detailsId : undefined}
          onClick={onOpen}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-control text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="min-w-0 flex-1 truncate">
            {span.service ? (
              <span className="text-muted-foreground">
                {highlight
                  ? highlightText(span.service, needle)
                  : span.service}{" "}
              </span>
            ) : null}
            <span
              className={
                matched === false ? "text-muted-foreground" : "text-foreground"
              }
            >
              {highlight ? highlightText(label, needle) : label}
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
          {bar ? (
            <div
              aria-hidden="true"
              className={cn(
                "absolute top-[9px] h-3 rounded-[2px]",
                span.error && "bg-status-critical",
                matched === false && "opacity-30",
                onPath &&
                  "outline outline-2 -outline-offset-2 outline-foreground"
              )}
              style={{
                left: `${bar.offset * 100}%`,
                width: `${bar.width * 100}%`,
                minWidth: 2,
                backgroundColor: span.error
                  ? undefined
                  : styleForSlot(span.slot).color,
              }}
            />
          ) : null}
        </div>
      </div>
    </div>
  )
}

const attributeGroups = [
  { source: "span", title: "Span attributes" },
  { source: "resource", title: "Resource attributes" },
] as const

function SpanAttributes({
  ref,
  id,
  span,
  className,
}: {
  ref?: Ref<HTMLDivElement>
  id?: string
  span: Span
  className: string
}) {
  const headingId = useId()
  const groups = attributeGroups.flatMap((group) => {
    const items = span.attributes.filter(
      (attribute) => attribute.source === group.source
    )
    return items.length === 0 ? [] : [{ ...group, items }]
  })
  return (
    <div
      ref={ref}
      id={id}
      className={cn("flex flex-col gap-2 text-xs", className)}
    >
      {groups.length === 0 ? (
        <p className="text-muted-foreground">No attributes.</p>
      ) : (
        groups.map((group) => (
          <div key={group.source} className="flex flex-col gap-1">
            <p
              id={`${headingId}-${group.source}`}
              className="text-xs font-medium text-muted-foreground"
            >
              {group.title}
            </p>
            <dl
              aria-labelledby={`${headingId}-${group.source}`}
              className="m-0 grid grid-cols-[minmax(5rem,max-content)_minmax(0,1fr)] gap-x-3 gap-y-0.5"
            >
              {group.items.map((attribute, index) => (
                <div key={`${index}:${attribute.key}`} className="contents">
                  <dt className="text-muted-foreground">{attribute.key}</dt>
                  <dd className="m-0 font-mono [overflow-wrap:anywhere] whitespace-pre-wrap">
                    {attribute.text}
                  </dd>
                </div>
              ))}
            </dl>
          </div>
        ))
      )}
    </div>
  )
}

function SpanPane({
  id,
  span,
  zone,
  formatDuration,
  onClose,
}: {
  id: string
  span: Span | undefined
  zone: string | undefined
  formatDuration: (ms: number) => string
  onClose: () => void
}) {
  return (
    <aside
      aria-label="Span details"
      id={id}
      className="w-[400px] max-w-1/2 shrink-0 overflow-auto border-l border-border p-2 text-xs"
    >
      {span ? (
        <>
          <div className="flex items-start justify-between gap-2">
            <p className="min-w-0 truncate text-sm font-medium text-foreground">
              {[span.service, span.operation].filter(Boolean).join(" ")}
            </p>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Close span details"
              onClick={onClose}
            >
              <XIcon aria-hidden="true" />
            </Button>
          </div>
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

function withoutIds(
  set: ReadonlySet<string>,
  ids: readonly string[]
): ReadonlySet<string> {
  if (!ids.some((id) => set.has(id))) return set
  const next = new Set(set)
  for (const id of ids) next.delete(id)
  return next
}

function rowSelector(id: string): string {
  return `[data-span-id="${CSS.escape(id)}"]`
}

function isFormField(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    target.closest("input, textarea, select, [contenteditable]") !== null
  )
}

export function TraceView(props: TraceViewProps) {
  const { model, filters, zone, width, height, attributes, toolbar } = props
  const [internalCollapsed, setInternalCollapsed] = useState<
    ReadonlySet<string>
  >(new Set())
  const [internalOpenId, setInternalOpenId] = useState<string | null>(null)
  const [focusedId, setFocusedId] = useState<string | null>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [detailHeight, setDetailHeight] = useState(0)
  const detailRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const scrolledTo = useRef<string | null>(null)
  const focusPending = useRef(false)
  const initialOpenId = useRef(props.openId ?? null)
  const detailsId = useId()

  const wanted = props.collapsed ?? internalCollapsed
  const setCollapsed = (next: ReadonlySet<string>) => {
    props.onCollapsedChange?.(next)
    if (!props.onCollapsedChange) setInternalCollapsed(next)
  }
  const openId = props.onOpenIdChange ? (props.openId ?? null) : internalOpenId
  const setOpenId = (id: string | null) => {
    props.onOpenIdChange?.(id)
    if (!props.onOpenIdChange) setInternalOpenId(id)
  }
  const zoom = props.zoom ?? wholeTrace
  const openAncestors = useMemo(
    () => (openId === null ? [] : ancestorIds(model.spans, openId)),
    [model.spans, openId]
  )
  const collapsed = useMemo(
    () => withoutIds(wanted, openAncestors),
    [wanted, openAncestors]
  )

  const filtered = useMemo(
    () => filterSpans(model.spans, filters),
    [model.spans, filters]
  )
  const rows = useMemo(
    () => visibleSpans(filtered.spans, collapsed),
    [filtered.spans, collapsed]
  )
  const windowMs = (zoom.end - zoom.start) * model.duration
  const ticks = niceTicks(windowMs)
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
  const columns = nameColumns(width)
  const matchedIds = props.highlighted ?? filtered.matched
  const needle = props.highlighted ? filters.search : ""
  const ruler = useZoomDrag(zoom, props.onZoom)

  useEffect(() => {
    const node = detailRef.current
    if (!node) return
    const observer = new ResizeObserver(() =>
      setDetailHeight(node.getBoundingClientRect().height)
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [openId, detailRendered])

  useEffect(() => {
    const list = listRef.current
    if (!list || openId === null || scrolledTo.current === openId) return
    const row = list.querySelector(rowSelector(openId))
    if (row) {
      row.scrollIntoView({
        block: openId === initialOpenId.current ? "center" : "nearest",
      })
      scrolledTo.current = openId
    } else if (openIndex >= 0) {
      list.scrollTop = Math.max(openIndex * rowHeight - height / 2, 0)
    }
  }, [openId, openIndex, range.start, range.end, height])

  useEffect(() => {
    const list = listRef.current
    if (!list || !focusPending.current || focusedId === null) return
    const row = list.querySelector<HTMLElement>(rowSelector(focusedId))
    if (row) {
      focusPending.current = false
      row.focus()
      row.scrollIntoView({ block: "nearest" })
    }
  }, [focusedId, range.start, range.end])

  const toggle = (id: string) => {
    const next = new Set(collapsed)
    if (!next.delete(id)) {
      next.add(id)
      if (openAncestors.includes(id)) setOpenId(null)
    }
    setCollapsed(next)
  }
  const open = (id: string) => {
    setFocusedId(id)
    setOpenId(openId === id ? null : id)
  }
  const focusRow = (index: number) => {
    const row = rows[Math.min(Math.max(index, 0), rows.length - 1)]
    if (!row) return
    focusPending.current = true
    setFocusedId(row.id)
    if (windowed) {
      const top = index * rowHeight
      const list = listRef.current
      if (
        list &&
        (top < list.scrollTop || top + rowHeight > list.scrollTop + height)
      ) {
        list.scrollTop = Math.max(top - height / 2, 0)
      }
    }
  }
  const tabStopId =
    [focusedId, openId].find(
      (id) => id !== null && rows.some((span) => span.id === id)
    ) ??
    rows[0]?.id ??
    null

  function onKeyDown(event: KeyboardEvent<HTMLUListElement>) {
    if (event.metaKey || event.ctrlKey || event.altKey) return
    if (isFormField(event.target)) return
    const target = event.target as HTMLElement
    const currentId =
      target.closest<HTMLElement>("[data-span-id]")?.dataset.spanId
    const current = rows.findIndex((span) => span.id === currentId)
    const span = current >= 0 ? rows[current] : undefined
    switch (event.key) {
      case "j":
      case "ArrowDown":
        focusRow(current + 1)
        break
      case "k":
      case "ArrowUp":
        focusRow(current <= 0 ? 0 : current - 1)
        break
      case "Enter":
        if (!span || target.getAttribute("role") !== "treeitem") return
        open(span.id)
        break
      case "Escape":
        if (openId === null) return
        setOpenId(null)
        break
      case "ArrowLeft":
        if (!span?.hasChildren || collapsed.has(span.id)) return
        toggle(span.id)
        break
      case "ArrowRight":
        if (!span?.hasChildren || !collapsed.has(span.id)) return
        toggle(span.id)
        break
      default:
        return
    }
    event.preventDefault()
  }

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
      {toolbar}
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
          ref={listRef}
          className="min-h-0 min-w-0 flex-1 overflow-auto"
          onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
        >
          <div
            className={cn(
              "sticky top-0 z-10 grid border-b border-border bg-background text-xs text-muted-foreground",
              columns
            )}
          >
            <span className="px-2 py-1">Service and operation</span>
            <div
              className={cn(
                "relative h-6 px-2",
                props.onZoom && "cursor-col-resize touch-none select-none"
              )}
              aria-hidden="true"
              {...ruler.handlers}
            >
              <div className="relative h-full">
                {ticks.map((tick) => (
                  <span
                    key={tick}
                    className={cn(
                      "absolute top-1 whitespace-nowrap tabular-nums",
                      tickClass(tick, windowMs)
                    )}
                    style={
                      tick === 0 || tick >= windowMs
                        ? undefined
                        : { left: `${(tick / windowMs) * 100}%` }
                    }
                  >
                    {model.formatDuration(zoom.start * model.duration + tick)}
                  </span>
                ))}
                <DragSelection selection={ruler.selection} />
              </div>
            </div>
          </div>
          <ul
            role="tree"
            aria-label={`Spans of trace ${model.traceId.slice(0, 8)}, ${filtered.spans.length.toLocaleString("en")} spans`}
            data-span-count={rows.length}
            className="m-0 list-none p-0"
            style={{ paddingTop: range.before, paddingBottom: range.after }}
            onKeyDown={onKeyDown}
          >
            {rows.slice(range.start, range.end).map((span) => {
              const isOpen = span.id === openId
              return (
                <li
                  key={span.id}
                  role="treeitem"
                  data-span-id={span.id}
                  aria-level={span.depth + 1}
                  aria-expanded={
                    span.hasChildren ? !collapsed.has(span.id) : undefined
                  }
                  aria-selected={isOpen}
                  tabIndex={span.id === tabStopId ? 0 : -1}
                  className="border-b border-border outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
                >
                  <SpanRow
                    span={span}
                    columns={columns}
                    collapsed={collapsed.has(span.id)}
                    open={isOpen}
                    matched={
                      matchedIds === null ? null : matchedIds.has(span.id)
                    }
                    needle={needle}
                    onPath={
                      props.criticalPathIds
                        ? props.criticalPathIds.has(span.id)
                        : null
                    }
                    dim={props.dimmed?.has(span.id) ?? false}
                    zoom={zoom}
                    detailsId={detailsId}
                    formatDuration={model.formatDuration}
                    onToggle={() => toggle(span.id)}
                    onOpen={() => open(span.id)}
                  />
                  {isOpen && inline ? (
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
            onClose={() => setOpenId(null)}
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
