import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  type Ref,
} from "react"
import { Link } from "@tanstack/react-router"
import {
  ChevronDownIcon,
  ChevronRightIcon,
  MinusIcon,
  PlusIcon,
  XIcon,
} from "lucide-react"

import { CopyButton } from "@/components/copy-button"
import { Button, buttonVariants } from "@/components/ui/button"
import { traceLink, tracesLink } from "@/lib/links"
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
import {
  foldedRows,
  foldLabel,
  foldPage,
  moreLabel,
  noGroups,
  revealSpan,
  rowId,
  type FoldGroup,
} from "./fold"
import { niceTicks } from "./ruler"
import {
  ancestorIds,
  filterSpans,
  spanFilters,
  type SpanFilters,
} from "./span-filters"
import {
  eventTicks,
  traceModel,
  windowRows,
  type Span,
  type SpanAttribute,
} from "./trace-model"

const rowHeight = 30
const windowAfter = 200
const overscan = 10
const indentStep = 12
const wideNameColumn = 600
const ErrorIcon = thresholdIcons.critical

export type TraceSearchLink = { uid: string; q: string }

export type SearchLink = (
  attribute: SpanAttribute,
  negate: boolean
) => TraceSearchLink | null

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
  header?: boolean
  searchLink?: SearchLink
  spanPaneExtra?: (span: Span) => ReactNode
  fold?: ReadonlyMap<string, FoldGroup>
  traceLinks?: TraceLinkTarget
}

export type TraceLinkTarget = { ds: string; from: string; to: string }

const noOpened: ReadonlyMap<string, number> = new Map()

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
  trace,
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
  trace: { start: number; duration: number }
  detailsId: string
  formatDuration: (ms: number) => string
  onToggle: () => void
  onOpen: () => void
}) {
  const CollapseIcon = collapsed ? ChevronRightIcon : ChevronDownIcon
  const label = span.operation || span.id
  const bar = zoomedOffset(span, zoom)
  const ticks = eventTicks(span.events, trace, zoom)
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
          {ticks.map((at, index) => (
            <span
              key={index}
              aria-hidden="true"
              className="absolute top-3 size-1.5 -translate-x-1/2 rounded-full bg-foreground"
              style={{ left: `${at * 100}%` }}
            />
          ))}
        </div>
      </div>
    </div>
  )
}

const foldHatch =
  "bg-[repeating-linear-gradient(45deg,var(--fold-color)_0_3px,transparent_3px_6px)]"

function FoldRow({
  group,
  columns,
  open,
  matched,
  needle,
  onPath,
  dim,
  zoom,
  formatDuration,
  onToggle,
}: {
  group: FoldGroup
  columns: string
  open: boolean
  matched: boolean | null
  needle: string
  onPath: boolean | null
  dim: boolean
  zoom: Zoom
  formatDuration: (ms: number) => string
  onToggle: () => void
}) {
  const Icon = open ? ChevronDownIcon : ChevronRightIcon
  const bar = zoomedOffset(group, zoom)
  const label = foldLabel(group, formatDuration)
  const count = group.count.toLocaleString("en")
  const highlight = matched && needle !== ""
  return (
    <div
      className={cn(
        "grid min-h-[30px] items-center text-xs",
        columns,
        (dim || onPath === false) && "opacity-[.42]"
      )}
      style={{ height: rowHeight - 1 }}
    >
      <div
        className="flex min-w-0 items-center gap-1 pr-2"
        style={{ paddingLeft: 8 + group.depth * indentStep }}
      >
        <button
          type="button"
          tabIndex={-1}
          aria-expanded={open}
          aria-label={`${open ? "Close" : "Open"} the ${count} folded ${group.operation} spans, slowest first`}
          onClick={onToggle}
          className="flex size-5 shrink-0 items-center justify-center rounded-control text-muted-foreground outline-none hover:bg-selection focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Icon className="size-3" aria-hidden="true" />
        </button>
        <button
          type="button"
          tabIndex={-1}
          onClick={onToggle}
          className="min-w-0 flex-1 truncate rounded-control text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {group.service ? (
            <span className="text-muted-foreground">
              {highlight
                ? highlightText(group.service, needle)
                : group.service}{" "}
            </span>
          ) : null}
          <span
            className={
              matched === false ? "text-muted-foreground" : "text-foreground"
            }
          >
            {highlight
              ? highlightText(group.operation, needle)
              : group.operation}
          </span>
          <span className="text-muted-foreground"> x {count} folded</span>
        </button>
      </div>
      <div className="h-full min-w-0 px-2" onClick={onToggle}>
        <div className="relative h-full">
          {bar ? (
            <>
              <div
                aria-hidden="true"
                className={cn(
                  "absolute top-[9px] h-3 rounded-[2px]",
                  foldHatch,
                  matched === false && "opacity-30",
                  onPath &&
                    "outline outline-2 -outline-offset-2 outline-foreground"
                )}
                style={
                  {
                    left: `${bar.offset * 100}%`,
                    width: `${bar.width * 100}%`,
                    minWidth: 2,
                    "--fold-color": styleForSlot(group.slot).color,
                  } as CSSProperties
                }
              >
                {group.errors > 0 ? (
                  <span className="absolute inset-y-0 left-0 w-0.5 rounded-l-[2px] bg-status-critical" />
                ) : null}
              </div>
              <span
                aria-hidden="true"
                className="absolute top-[6px] max-w-full truncate rounded-[2px] bg-background/80 px-1 leading-4 text-muted-foreground tabular-nums"
                style={{
                  left: `min(${bar.offset * 100}%, calc(100% - 12rem))`,
                }}
              >
                {label}
              </span>
            </>
          ) : null}
        </div>
      </div>
    </div>
  )
}

function MoreRow({
  group,
  shown,
  columns,
  onMore,
}: {
  group: FoldGroup
  shown: number
  columns: string
  onMore: () => void
}) {
  return (
    <div
      className={cn("grid items-center text-xs", columns)}
      style={{ height: rowHeight - 1 }}
    >
      <div
        className="flex min-w-0 items-center"
        style={{ paddingLeft: 8 + group.depth * indentStep + 20 }}
      >
        <Button
          type="button"
          variant="ghost"
          size="xs"
          tabIndex={-1}
          onClick={onMore}
        >
          {moreLabel(group, shown)}
        </Button>
      </div>
      <div className="h-full" />
    </div>
  )
}

function kindLine(span: Span): string {
  return [
    span.service,
    span.kind === null ? "" : `${span.kind.toLowerCase()} span`,
  ]
    .filter(Boolean)
    .join(" / ")
}

const kv3Columns = "grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)_auto]"

function SectionHeading({ title, count }: { title: string; count: number }) {
  return (
    <h3 className="m-0 text-xs font-semibold text-foreground">
      {title}{" "}
      <span className="font-normal text-muted-foreground">
        {count.toLocaleString("en")}
      </span>
    </h3>
  )
}

function EventList({
  span,
  formatDuration,
}: {
  span: Span
  formatDuration: (ms: number) => string
}) {
  if (span.events.length === 0) return null
  return (
    <section className="flex flex-col gap-1">
      <SectionHeading title="Events" count={span.events.length} />
      <ol className="m-0 flex list-none flex-col gap-1.5 p-0">
        {span.events.map((event, index) => (
          <li
            key={index}
            className="flex flex-col gap-0.5 rounded-control border border-border px-2 py-1.5"
          >
            <div className="flex justify-between gap-2">
              <span className="font-medium [overflow-wrap:anywhere]">
                {event.name}
              </span>
              <span className="shrink-0 text-muted-foreground tabular-nums">
                +{formatDuration(event.time - span.start)}
              </span>
            </div>
            <TextPairRows pairs={event.attributes} />
          </li>
        ))}
      </ol>
    </section>
  )
}

function TextPairRows({ pairs }: { pairs: Record<string, string> }) {
  return Object.entries(pairs).map(([key, value]) => (
    <div
      key={key}
      className={cn("grid min-h-6 items-center gap-x-2", kv3Columns)}
    >
      <span className="font-mono [overflow-wrap:anywhere] text-muted-foreground">
        {key}
      </span>
      <span
        className={cn(
          "col-span-2 font-mono [overflow-wrap:anywhere]",
          key === "exception.message" && "whitespace-pre-wrap"
        )}
      >
        {value}
      </span>
    </div>
  ))
}

function LinkList({
  span,
  target,
}: {
  span: Span
  target: TraceLinkTarget | undefined
}) {
  if (span.links.length === 0) return null
  return (
    <section className="flex flex-col gap-1">
      <SectionHeading title="Links" count={span.links.length} />
      <ol className="m-0 flex list-none flex-col gap-1.5 p-0">
        {span.links.map((link, index) => (
          <li
            key={index}
            className="flex flex-col gap-0.5 rounded-control border border-border px-2 py-1.5"
          >
            <div className="flex flex-wrap gap-x-2 gap-y-0.5">
              <span className="text-muted-foreground">Trace</span>
              {target ? (
                <Link
                  {...traceLink(link.traceId, target.ds, target, link.spanId)}
                  className="font-mono [overflow-wrap:anywhere] underline-offset-3 hover:underline"
                >
                  {link.traceId}
                </Link>
              ) : (
                <span className="font-mono [overflow-wrap:anywhere]">
                  {link.traceId}
                </span>
              )}
              <span className="text-muted-foreground">Span</span>
              <span className="font-mono [overflow-wrap:anywhere]">
                {link.spanId}
              </span>
            </div>
            <TextPairRows pairs={link.attributes} />
          </li>
        ))}
      </ol>
    </section>
  )
}

function SearchAction({
  link,
  label,
  children,
}: {
  link: TraceSearchLink
  label: string
  children: ReactNode
}) {
  return (
    <Link
      {...tracesLink(link.uid, { q: link.q })}
      aria-label={label}
      className={cn(
        buttonVariants({ variant: "ghost", size: "icon-xs" }),
        "size-5"
      )}
    >
      {children}
    </Link>
  )
}

function AttributeRow({
  attribute,
  searchLink,
}: {
  attribute: SpanAttribute
  searchLink: SearchLink | undefined
}) {
  const include = searchLink?.(attribute, false) ?? null
  const exclude = searchLink?.(attribute, true) ?? null
  return (
    <div className={cn("group grid min-h-7 items-center gap-x-2", kv3Columns)}>
      <span className="font-mono [overflow-wrap:anywhere] text-muted-foreground">
        {attribute.key}
      </span>
      <span className="font-mono [overflow-wrap:anywhere] whitespace-pre-wrap">
        {attribute.text}
      </span>
      <span className="flex items-center gap-0.5 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100">
        {include ? (
          <SearchAction
            link={include}
            label={`Search traces where ${attribute.key} is ${attribute.text}`}
          >
            <PlusIcon aria-hidden="true" />
          </SearchAction>
        ) : null}
        {exclude ? (
          <SearchAction
            link={exclude}
            label={`Search traces where ${attribute.key} is not ${attribute.text}`}
          >
            <MinusIcon aria-hidden="true" />
          </SearchAction>
        ) : null}
        <CopyButton
          variant="ghost"
          size="icon-xs"
          className="size-5"
          text={attribute.text}
          label={`Copy value ${attribute.text}`}
        />
      </span>
    </div>
  )
}

function AttributeSection({
  title,
  items,
  searchLink,
}: {
  title: string
  items: SpanAttribute[]
  searchLink: SearchLink | undefined
}) {
  if (items.length === 0) return null
  return (
    <section className="flex flex-col gap-0.5">
      <SectionHeading title={title} count={items.length} />
      {items.map((attribute, index) => (
        <AttributeRow
          key={`${index}:${attribute.key}`}
          attribute={attribute}
          searchLink={searchLink}
        />
      ))}
    </section>
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
  formatDuration,
  className,
}: {
  ref?: Ref<HTMLDivElement>
  id?: string
  span: Span
  formatDuration: (ms: number) => string
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
      {span.kind !== null ? (
        <p className="text-muted-foreground">{kindLine(span)}</p>
      ) : null}
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
      <EventList span={span} formatDuration={formatDuration} />
    </div>
  )
}

function SpanPane({
  id,
  span,
  model,
  searchLink,
  traceLinks,
  extra,
  onClose,
}: {
  id: string
  span: Span | undefined
  model: TraceViewProps["model"]
  searchLink: SearchLink | undefined
  traceLinks: TraceLinkTarget | undefined
  extra: ReactNode
  onClose: () => void
}) {
  const share =
    span && model.duration > 0
      ? Math.round((span.duration / model.duration) * 100)
      : 0
  const spanAttributes =
    span?.attributes.filter((attribute) => attribute.source === "span") ?? []
  const resourceAttributes =
    span?.attributes.filter((attribute) => attribute.source === "resource") ??
    []
  return (
    <aside
      aria-label="Span details"
      id={id}
      className="w-[400px] max-w-1/2 shrink-0 overflow-auto border-l border-border p-3 text-xs"
    >
      {span ? (
        <div className="flex flex-col gap-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex min-w-0 flex-col gap-0.5">
              <p className="text-muted-foreground">{kindLine(span)}</p>
              <h2 className="m-0 text-[13px] font-semibold [overflow-wrap:anywhere] text-foreground">
                {span.operation || span.id}
              </h2>
              <p className="flex flex-wrap gap-x-2">
                <span
                  className={cn(
                    "font-medium",
                    span.error ? "text-status-critical" : "text-foreground"
                  )}
                >
                  {span.error ? "Error" : "OK"}
                </span>
                <span className="tabular-nums">
                  {model.formatDuration(span.duration)}
                </span>
                <span className="text-muted-foreground tabular-nums">
                  {share}% of the trace
                </span>
                <span className="text-muted-foreground tabular-nums">
                  starts at +{model.formatDuration(span.start - model.start)}
                </span>
              </p>
            </div>
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
          {span.attributes.length === 0 ? (
            <p className="text-muted-foreground">No attributes.</p>
          ) : null}
          <AttributeSection
            title="Attributes"
            items={spanAttributes}
            searchLink={searchLink}
          />
          <AttributeSection
            title="Resource"
            items={resourceAttributes}
            searchLink={searchLink}
          />
          <EventList span={span} formatDuration={model.formatDuration} />
          <LinkList span={span} target={traceLinks} />
          {extra}
        </div>
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
  const groups = props.fold ?? noGroups
  const [opened, setOpened] = useState<ReadonlyMap<string, number>>(noOpened)
  const openAncestors = useMemo(
    () => (openId === null ? [] : ancestorIds(model.spans, openId)),
    [model.spans, openId]
  )
  const collapsed = useMemo(
    () => withoutIds(wanted, openAncestors),
    [wanted, openAncestors]
  )
  const shown = useMemo(
    () =>
      openId === null
        ? opened
        : revealSpan(opened, groups, model.spans, openId),
    [opened, groups, model.spans, openId]
  )

  const filtered = useMemo(
    () => filterSpans(model.spans, filters),
    [model.spans, filters]
  )
  const rows = useMemo(
    () => foldedRows(filtered.spans, groups, collapsed, shown),
    [filtered.spans, groups, collapsed, shown]
  )
  const windowMs = (zoom.end - zoom.start) * model.duration
  const ticks = niceTicks(windowMs)
  const openIndex = rows.findIndex(
    (row) => row.kind === "span" && row.span.id === openId
  )
  const openRow = openIndex === -1 ? undefined : rows[openIndex]
  const openSpan = openRow?.kind === "span" ? openRow.span : undefined
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
  const showFold = (group: FoldGroup, count: number) => {
    const next = new Map(shown)
    if (count > 0) {
      next.set(group.id, count)
    } else {
      next.delete(group.id)
      const holdsOpen = group.spans.some(
        (span) => span.id === openId || openAncestors.includes(span.id)
      )
      if (holdsOpen) setOpenId(null)
    }
    setOpened(next)
  }
  const foldOpen = (group: FoldGroup) => (shown.get(group.id) ?? 0) > 0
  const toggleFold = (group: FoldGroup) =>
    showFold(group, foldOpen(group) ? 0 : foldPage)
  const focusRow = (index: number) => {
    const row = rows[Math.min(Math.max(index, 0), rows.length - 1)]
    if (!row) return
    focusPending.current = true
    setFocusedId(rowId(row))
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
      (id) => id !== null && rows.some((row) => rowId(row) === id)
    ) ?? (rows[0] ? rowId(rows[0]) : null)

  function onKeyDown(event: KeyboardEvent<HTMLUListElement>) {
    if (event.metaKey || event.ctrlKey || event.altKey) return
    if (isFormField(event.target)) return
    const target = event.target as HTMLElement
    const currentId =
      target.closest<HTMLElement>("[data-span-id]")?.dataset.spanId
    const current = rows.findIndex((row) => rowId(row) === currentId)
    const row = current >= 0 ? rows[current] : undefined
    const span = row?.kind === "span" ? row.span : undefined
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
        if (!row || target.getAttribute("role") !== "treeitem") return
        if (row.kind === "span") open(row.span.id)
        else if (row.kind === "fold") toggleFold(row.group)
        else showFold(row.group, row.shown + foldPage)
        break
      case "Escape":
        if (openId === null) return
        setOpenId(null)
        break
      case "ArrowLeft":
        if (row?.kind === "fold") {
          if (!foldOpen(row.group)) return
          showFold(row.group, 0)
          break
        }
        if (!span?.hasChildren || collapsed.has(span.id)) return
        toggle(span.id)
        break
      case "ArrowRight":
        if (row?.kind === "fold") {
          if (foldOpen(row.group)) return
          showFold(row.group, foldPage)
          break
        }
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
      {props.header === false ? null : (
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
      )}
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
            {rows.slice(range.start, range.end).map((row) => {
              const id = rowId(row)
              const itemClass =
                "border-b border-border outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
              if (row.kind === "fold") {
                const { group } = row
                const isOpen = foldOpen(group)
                return (
                  <li
                    key={id}
                    role="treeitem"
                    data-span-id={id}
                    aria-level={group.depth + 1}
                    aria-expanded={isOpen}
                    aria-selected={false}
                    aria-label={`${group.operation}, ${group.count.toLocaleString("en")} folded spans, ${foldLabel(group, model.formatDuration)}`}
                    tabIndex={id === tabStopId ? 0 : -1}
                    className={itemClass}
                  >
                    <FoldRow
                      group={group}
                      columns={columns}
                      open={isOpen}
                      matched={
                        matchedIds === null
                          ? null
                          : group.spans.some((span) => matchedIds.has(span.id))
                      }
                      needle={needle}
                      onPath={
                        props.criticalPathIds
                          ? group.spans.some((span) =>
                              props.criticalPathIds!.has(span.id)
                            )
                          : null
                      }
                      dim={
                        props.dimmed
                          ? group.spans.every((span) =>
                              props.dimmed!.has(span.id)
                            )
                          : false
                      }
                      zoom={zoom}
                      formatDuration={model.formatDuration}
                      onToggle={() => toggleFold(group)}
                    />
                  </li>
                )
              }
              if (row.kind === "more") {
                const { group } = row
                return (
                  <li
                    key={id}
                    role="treeitem"
                    data-span-id={id}
                    aria-level={group.depth + 1}
                    aria-selected={false}
                    aria-label={`${moreLabel(group, row.shown)} folded ${group.operation} spans`}
                    tabIndex={id === tabStopId ? 0 : -1}
                    className={itemClass}
                  >
                    <MoreRow
                      group={group}
                      shown={row.shown}
                      columns={columns}
                      onMore={() => showFold(group, row.shown + foldPage)}
                    />
                  </li>
                )
              }
              const { span } = row
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
                  className={itemClass}
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
                    trace={model}
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
                      formatDuration={model.formatDuration}
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
            span={openSpan}
            model={model}
            searchLink={props.searchLink}
            traceLinks={props.traceLinks}
            extra={
              openSpan && props.spanPaneExtra
                ? props.spanPaneExtra(openSpan)
                : null
            }
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
