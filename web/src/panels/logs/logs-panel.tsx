import {
  memo,
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"

import type { FieldResolver } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import { logWindow, rowOffsets } from "./log-window"
import {
  detailText,
  formatLogTime,
  logsModel,
  maxEntries,
  messageTokens,
  showsAttributes,
  type DedupStrategy,
  type Labels,
  type LogDetail,
  type LogEntry,
  type LogLevelKind,
  type LogsDisplay,
  type MessageToken,
  type TimePrecision,
} from "./logs-model"

const levelIcons = {
  error: thresholdIcons.critical,
  warning: thresholdIcons.warning,
  info: thresholdIcons.info,
} as const

const levelColors: Record<LogLevelKind, string> = {
  error: "text-status-critical",
  warning: "text-status-warning",
  info: "text-threshold-info",
  other: "",
}

const tokenClasses: Record<MessageToken["kind"], string> = {
  text: "",
  key: "text-syntax-keyword",
  string: "text-syntax-string",
  number: "text-syntax-number",
  level: "font-semibold",
}

const dedupOptions: { value: DedupStrategy; label: string }[] = [
  { value: "none", label: "No deduplication" },
  { value: "exact", label: "Exact" },
  { value: "numbers", label: "Ignore numbers" },
  { value: "signature", label: "Ignore letters and numbers" },
]

type ViewState = Pick<LogsDisplay, "order" | "wrap" | "showTime" | "dedup">

const orderOptions: { value: ViewState["order"]; label: string }[] = [
  { value: "newest", label: "Newest first" },
  { value: "oldest", label: "Oldest first" },
]

export type LogSelection = {
  entry: LogEntry
  streamLabels: Labels
  fields: FieldResolver
}

type DetailActions = (selection: LogSelection) => ReactNode

const windowAfter = 200
const overscan = 10

type DetailsMode = "off" | "inline" | "sidebar" | "external"

function labelText(labels: Labels): string {
  return Object.entries(labels)
    .map(([name, value]) => `${name}=${value}`)
    .join(" ")
}

function Message({ text, highlight }: { text: string; highlight: boolean }) {
  const tokens = useMemo(
    () => (highlight ? messageTokens(text) : null),
    [text, highlight]
  )
  if (!tokens) return text
  return tokens.map((token, index) => (
    <span key={index} className={tokenClasses[token.kind]}>
      {token.text}
    </span>
  ))
}

export function LogDetails({
  details,
  fields,
  zone,
  id,
  actions,
  layout = "panel",
}: {
  details: readonly LogDetail[]
  fields: FieldResolver
  zone: string | undefined
  id?: string
  actions?: ReactNode
  layout?: "panel" | "drawer"
}) {
  return (
    <>
      {actions ? (
        <div
          role="group"
          aria-label="Line actions"
          className="flex flex-wrap gap-2 px-2 pt-1.5"
        >
          {actions}
        </div>
      ) : null}
      <dl
        id={id}
        className={cn(
          "grid gap-x-3 gap-y-0.5",
          layout === "drawer"
            ? "grid-cols-[minmax(0,8rem)_minmax(0,1fr)] text-xs"
            : "grid-cols-[minmax(5rem,max-content)_minmax(0,1fr)] px-2 pt-0.5 pb-2"
        )}
      >
        {details.map((detail, index) => (
          <div key={`${index}:${detail.name}`} className="contents">
            <dt
              className={cn(
                "text-muted-foreground",
                layout === "drawer" && "font-mono [overflow-wrap:anywhere]"
              )}
            >
              {detail.name}
            </dt>
            <dd className="font-mono [overflow-wrap:anywhere] whitespace-pre-wrap">
              {detailText(fields, detail, zone)}
            </dd>
          </div>
        ))}
      </dl>
    </>
  )
}

type RowProps = {
  entry: LogEntry
  fields: FieldResolver
  zone: string | undefined
  wrap: boolean
  showTime: boolean
  showLabels: boolean
  timePrecision: TimePrecision | undefined
  highlight: boolean
  levelColumn: boolean
  detailsMode: DetailsMode
  selected: boolean
  controlsId: string
  onSelect: (key: string | null) => void
  marked: boolean
  detailActions: DetailActions | undefined
  commonLabels: Labels
  setSize: number | undefined
  posInSet: number | undefined
}

const rowButtonClasses =
  "flex w-full items-start gap-2 px-2 py-1 text-left outline-none"

const LogRow = memo(function LogRow({
  entry,
  fields,
  zone,
  wrap,
  showTime,
  showLabels,
  timePrecision,
  highlight,
  levelColumn,
  detailsMode,
  selected,
  controlsId,
  onSelect,
  marked,
  detailActions,
  commonLabels,
  setSize,
  posInSet,
}: RowProps) {
  const [open, setOpen] = useState(false)
  const detailsId = useId()
  const kind = entry.level?.kind ?? "other"
  const Icon = kind === "other" ? null : levelIcons[kind]
  const controlled = detailsMode === "sidebar" || detailsMode === "external"
  const expanded = controlled ? selected : open
  const wraps = wrap || entry.message.includes("\n")
  const line = (
    <>
      {showTime ? (
        <span className="shrink-0 leading-5 text-muted-foreground tabular-nums">
          {entry.time === null ? (
            "-"
          ) : (
            <time dateTime={new Date(entry.time).toISOString()}>
              {formatLogTime(entry.time, timePrecision, zone)}
            </time>
          )}
        </span>
      ) : null}
      {showLabels && Object.keys(entry.labels).length > 0 ? (
        <span
          data-logs-labels=""
          className="max-w-[40%] shrink-0 truncate font-mono leading-5 text-muted-foreground"
        >
          {labelText(entry.labels)}
        </span>
      ) : null}
      {levelColumn ? (
        <span
          data-logs-level=""
          title={entry.level?.word}
          className={cn(
            "flex w-[4.5rem] shrink-0 items-center gap-1 leading-5",
            kind === "other" ? "text-muted-foreground" : "text-foreground"
          )}
        >
          {Icon ? (
            <Icon
              className={cn("size-3 shrink-0", levelColors[kind])}
              aria-hidden="true"
            />
          ) : null}
          <span className="truncate">{entry.level?.word}</span>
        </span>
      ) : null}
      <span
        data-logs-message=""
        className={cn(
          "min-w-0 flex-1 font-mono leading-5 text-foreground",
          wraps ? "[overflow-wrap:anywhere] whitespace-pre-wrap" : "truncate"
        )}
      >
        <Message text={entry.message} highlight={highlight} />
      </span>
      {marked ? <span className="sr-only">Selected line</span> : null}
      {entry.repeats > 1 ? (
        <span
          data-logs-repeats=""
          className="shrink-0 leading-5 text-muted-foreground tabular-nums"
        >
          Repeated {entry.repeats} times
        </span>
      ) : null}
    </>
  )
  return (
    <li
      data-level={kind}
      data-key={entry.key}
      data-marked={marked ? "" : undefined}
      aria-setsize={setSize}
      aria-posinset={posInSet}
      className={cn(
        "border-b border-border",
        setSize === undefined &&
          "[contain-intrinsic-size:auto_28px] [content-visibility:auto]",
        marked && "bg-selection"
      )}
    >
      {detailsMode === "off" ? (
        <div className={rowButtonClasses}>{line}</div>
      ) : (
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={
            expanded ? (controlled ? controlsId : detailsId) : undefined
          }
          onClick={() =>
            controlled
              ? onSelect(selected ? null : entry.key)
              : setOpen((current) => !current)
          }
          className={cn(
            rowButtonClasses,
            "hover:bg-selection focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset aria-expanded:bg-selection"
          )}
        >
          {line}
        </button>
      )}
      {detailsMode === "inline" && open ? (
        <LogDetails
          details={entry.details}
          fields={fields}
          zone={zone}
          id={detailsId}
          actions={detailActions?.({
            entry,
            streamLabels: { ...commonLabels, ...entry.labels },
            fields,
          })}
        />
      ) : null}
    </li>
  )
})

function ControlsBar({
  view,
  onChange,
}: {
  view: ViewState
  onChange: (change: Partial<ViewState>) => void
}) {
  const orderId = useId()
  const dedupId = useId()
  return (
    <div
      role="toolbar"
      aria-label="Log controls"
      className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 border-b border-border px-2 py-1.5"
    >
      <div className="flex items-center gap-2">
        <Label htmlFor={orderId} className="text-[13px] font-normal">
          Order
        </Label>
        <Select
          items={orderOptions}
          value={view.order}
          onValueChange={(order) => {
            if (order !== null) onChange({ order })
          }}
        >
          <SelectTrigger id={orderId}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {orderOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Label className="gap-2 text-[13px] font-normal">
        <Checkbox
          checked={view.wrap}
          onCheckedChange={(wrap) => onChange({ wrap })}
        />
        Wrap lines
      </Label>
      <Label className="gap-2 text-[13px] font-normal">
        <Checkbox
          checked={view.showTime}
          onCheckedChange={(showTime) => onChange({ showTime })}
        />
        Show time
      </Label>
      <div className="flex items-center gap-2">
        <Label htmlFor={dedupId} className="text-[13px] font-normal">
          Deduplication
        </Label>
        <Select
          items={dedupOptions}
          value={view.dedup}
          onValueChange={(dedup) => {
            if (dedup !== null) onChange({ dedup })
          }}
        >
          <SelectTrigger id={dedupId}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {dedupOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}

export type LogsViewProps = {
  panelTitle: string
  frames: Parameters<typeof logsModel>[0]
  fields: FieldResolver
  zone: string | undefined
  display: LogsDisplay
  width: number
  height: number
  detailActions?: DetailActions
  isMarked?: (entry: LogEntry) => boolean
  selectedKey?: string | null
  onSelectedChange?: (selection: LogSelection | null) => void
  detailsId?: string
}

function useRowHeights(enabled: boolean) {
  const [heights, setHeights] = useState<ReadonlyMap<string, number>>(
    () => new Map()
  )
  const observerRef = useRef<ResizeObserver | null>(null)
  const observed = useRef(new Set<Element>())
  useEffect(() => {
    if (!enabled) return
    const pending = new Map<string, number>()
    let frame = 0
    const observer = new ResizeObserver((records) => {
      for (const record of records) {
        const key = (record.target as HTMLElement).dataset.key
        const height = record.borderBoxSize?.[0]?.blockSize
        if (key !== undefined && height !== undefined) pending.set(key, height)
      }
      if (frame !== 0 || pending.size === 0) return
      frame = requestAnimationFrame(() => {
        frame = 0
        const batch = new Map(pending)
        pending.clear()
        setHeights((current) => {
          let changed = false
          for (const [key, height] of batch) {
            if (current.get(key) !== height) changed = true
          }
          return changed ? new Map([...current, ...batch]) : current
        })
      })
    })
    const elements = observed.current
    observerRef.current = observer
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
      elements.clear()
      observerRef.current = null
    }
  }, [enabled])
  const observeRows = useCallback((list: HTMLUListElement | null) => {
    const observer = observerRef.current
    if (!observer || !list) return
    for (const element of observed.current) {
      if (!element.isConnected) {
        observer.unobserve(element)
        observed.current.delete(element)
      }
    }
    for (const element of list.children) {
      if (!observed.current.has(element)) {
        observer.observe(element)
        observed.current.add(element)
      }
    }
  }, [])
  return { heights, observeRows }
}

function useViewportHeight(element: HTMLDivElement | null) {
  const [height, setHeight] = useState(0)
  useEffect(() => {
    if (!element) return
    const observer = new ResizeObserver(([record]) =>
      setHeight(record.contentRect.height)
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [element])
  return height
}

export function LogsView({
  panelTitle,
  frames,
  fields,
  zone,
  display,
  width,
  height,
  detailActions,
  isMarked,
  selectedKey: controlledKey = null,
  onSelectedChange,
  detailsId,
}: LogsViewProps) {
  const [view, setView] = useState<ViewState>({
    order: display.order,
    wrap: display.wrap,
    showTime: display.showTime,
    dedup: display.dedup,
  })
  const [internalKey, setInternalKey] = useState<string | null>(null)
  const sidebarId = useId()
  const change = useCallback(
    (next: Partial<ViewState>) =>
      setView((current) => ({ ...current, ...next })),
    []
  )
  const { entries, commonLabels, truncated } = useMemo(
    () =>
      logsModel(
        frames,
        fields,
        {
          order: view.order,
          dedup: view.dedup,
          prettify: display.prettify,
          lineFields: display.lineFields,
        },
        zone
      ),
    [
      frames,
      fields,
      zone,
      view.order,
      view.dedup,
      display.prettify,
      display.lineFields,
    ]
  )
  const levelColumn = useMemo(
    () => entries.some((entry) => entry.level !== null),
    [entries]
  )
  const detailsMode: DetailsMode = !display.details
    ? "off"
    : onSelectedChange
      ? "external"
      : display.detailsMode === "sidebar"
        ? "sidebar"
        : "inline"
  const selected = entries.find(
    (entry) =>
      entry.key === (detailsMode === "external" ? controlledKey : internalKey)
  )
  const select = useCallback(
    (key: string | null) => {
      if (!onSelectedChange) {
        setInternalKey(key)
        return
      }
      const entry = entries.find((candidate) => candidate.key === key)
      onSelectedChange(
        entry
          ? {
              entry,
              streamLabels: { ...commonLabels, ...entry.labels },
              fields,
            }
          : null
      )
    },
    [onSelectedChange, entries, commonLabels, fields]
  )
  const commonText = labelText(commonLabels)
  const windowed = entries.length > windowAfter
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(
    null
  )
  const [scrollTop, setScrollTop] = useState(0)
  const viewport = useViewportHeight(scrollElement)
  const { heights, observeRows } = useRowHeights(windowed)
  const listRef = useRef<HTMLUListElement>(null)
  const keys = useMemo(() => entries.map((entry) => entry.key), [entries])
  const offsets = useMemo(
    () => (windowed ? rowOffsets(keys, heights) : []),
    [windowed, keys, heights]
  )
  const range = windowed
    ? logWindow({ offsets, scrollTop, viewport, overscan })
    : { start: 0, end: entries.length, before: 0, after: 0 }
  const firstMarkedKey = isMarked ? entries.find(isMarked)?.key : undefined

  useEffect(() => {
    if (windowed) observeRows(listRef.current)
  })

  useEffect(() => {
    if (firstMarkedKey === undefined) return
    listRef.current
      ?.querySelector<HTMLElement>("[data-marked]")
      ?.scrollIntoView({ block: "center" })
  }, [firstMarkedKey])

  return (
    <div
      className={cn(
        "flex flex-col",
        display.fontSize === "small" ? "text-[11px]" : "text-xs"
      )}
      style={{ width, height }}
    >
      {display.controls ? <ControlsBar view={view} onChange={change} /> : null}
      {display.showCommonLabels && commonText !== "" ? (
        <p
          data-logs-common-labels=""
          className="shrink-0 border-b border-border px-2 py-1.5"
        >
          <span className="text-muted-foreground">Common labels </span>
          <span className="font-mono [overflow-wrap:anywhere]">
            {commonText}
          </span>
        </p>
      ) : null}
      {truncated ? (
        <p className="shrink-0 border-b border-border px-2 py-1.5 text-muted-foreground">
          Showing the {view.order === "oldest" ? "oldest" : "newest"}{" "}
          {maxEntries.toLocaleString("en")} lines.
        </p>
      ) : null}
      <div className="flex min-h-0 flex-1">
        <div
          ref={setScrollElement}
          className="min-h-0 min-w-0 flex-1 overflow-auto"
          onScroll={
            windowed
              ? (event) => setScrollTop(event.currentTarget.scrollTop)
              : undefined
          }
        >
          <ul
            ref={listRef}
            role="list"
            aria-label={`${panelTitle} log lines`}
            data-logs-count={entries.length}
            className="m-0 list-none p-0"
            style={
              windowed
                ? { paddingTop: range.before, paddingBottom: range.after }
                : undefined
            }
          >
            {entries.slice(range.start, range.end).map((entry, offset) => (
              <LogRow
                key={entry.key}
                entry={entry}
                fields={fields}
                zone={zone}
                wrap={view.wrap}
                showTime={view.showTime}
                showLabels={showsAttributes(display)}
                timePrecision={display.timePrecision}
                highlight={display.highlight}
                levelColumn={levelColumn}
                detailsMode={detailsMode}
                selected={entry.key === selected?.key}
                controlsId={detailsId ?? sidebarId}
                onSelect={select}
                marked={isMarked?.(entry) ?? false}
                detailActions={detailActions}
                commonLabels={commonLabels}
                setSize={windowed ? entries.length : undefined}
                posInSet={windowed ? range.start + offset + 1 : undefined}
              />
            ))}
          </ul>
        </div>
        {detailsMode === "sidebar" && selected ? (
          <aside
            aria-label="Log line details"
            className="min-w-48 shrink-0 basis-2/5 overflow-auto border-l border-border"
          >
            <LogDetails
              details={selected.details}
              fields={fields}
              zone={zone}
              id={sidebarId}
              actions={detailActions?.({
                entry: selected,
                streamLabels: { ...commonLabels, ...selected.labels },
                fields,
              })}
            />
          </aside>
        ) : null}
      </div>
    </div>
  )
}

const LogsPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  zone,
  display: rawDisplay,
  width,
  height,
}) => {
  const display = rawDisplay as LogsDisplay
  const settings = [
    display.order,
    display.wrap,
    display.showTime,
    display.dedup,
  ].join("|")
  return (
    <LogsView
      key={settings}
      panelTitle={panel.title}
      frames={frames}
      fields={fields}
      zone={zone}
      display={display}
      width={width}
      height={height}
    />
  )
}

export default LogsPanel
