import { memo, useCallback, useId, useMemo, useState } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { formatDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"

import type { FieldResolver } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import {
  detailText,
  logsModel,
  maxEntries,
  messageTokens,
  type DedupStrategy,
  type Labels,
  type LogEntry,
  type LogLevelKind,
  type LogsDisplay,
  type MessageToken,
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

type DetailsMode = "off" | "inline" | "sidebar"

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

function LogDetails({
  entry,
  fields,
  id,
}: {
  entry: LogEntry
  fields: FieldResolver
  id: string
}) {
  return (
    <dl
      id={id}
      className="grid grid-cols-[minmax(5rem,max-content)_minmax(0,1fr)] gap-x-3 gap-y-0.5 px-2 pt-0.5 pb-2"
    >
      {entry.details.map((detail, index) => (
        <div key={`${index}:${detail.name}`} className="contents">
          <dt className="text-muted-foreground">{detail.name}</dt>
          <dd className="font-mono [overflow-wrap:anywhere] whitespace-pre-wrap">
            {detailText(fields, detail)}
          </dd>
        </div>
      ))}
    </dl>
  )
}

type RowProps = {
  entry: LogEntry
  fields: FieldResolver
  wrap: boolean
  showTime: boolean
  showLabels: boolean
  highlight: boolean
  levelColumn: boolean
  detailsMode: DetailsMode
  selected: boolean
  sidebarId: string
  onSelect: (key: string | null) => void
}

const rowButtonClasses =
  "flex w-full items-start gap-2 px-2 py-1 text-left outline-none"

const LogRow = memo(function LogRow({
  entry,
  fields,
  wrap,
  showTime,
  showLabels,
  highlight,
  levelColumn,
  detailsMode,
  selected,
  sidebarId,
  onSelect,
}: RowProps) {
  const [open, setOpen] = useState(false)
  const detailsId = useId()
  const kind = entry.level?.kind ?? "other"
  const Icon = kind === "other" ? null : levelIcons[kind]
  const expanded = detailsMode === "sidebar" ? selected : open
  const wraps = wrap || entry.message.includes("\n")
  const line = (
    <>
      {showTime ? (
        <span className="shrink-0 leading-5 text-muted-foreground tabular-nums">
          {entry.time === null ? (
            "-"
          ) : (
            <time dateTime={new Date(entry.time).toISOString()}>
              {formatDateTimeMs(entry.time)}
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
      className="border-b border-border [contain-intrinsic-size:auto_28px] [content-visibility:auto]"
    >
      {detailsMode === "off" ? (
        <div className={rowButtonClasses}>{line}</div>
      ) : (
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={
            expanded
              ? detailsMode === "sidebar"
                ? sidebarId
                : detailsId
              : undefined
          }
          onClick={() =>
            detailsMode === "sidebar"
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
        <LogDetails entry={entry} fields={fields} id={detailsId} />
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
  const selectClasses =
    "min-h-8 rounded-control border border-input bg-background px-2 text-[13px] text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
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
        <select
          id={orderId}
          value={view.order}
          onChange={(event) =>
            onChange({ order: event.target.value as ViewState["order"] })
          }
          className={selectClasses}
        >
          <option value="newest">Newest first</option>
          <option value="oldest">Oldest first</option>
        </select>
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
        <select
          id={dedupId}
          value={view.dedup}
          onChange={(event) =>
            onChange({ dedup: event.target.value as DedupStrategy })
          }
          className={selectClasses}
        >
          {dedupOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </div>
    </div>
  )
}

type ViewProps = {
  panelTitle: string
  frames: Parameters<typeof logsModel>[0]
  fields: FieldResolver
  display: LogsDisplay
  width: number
  height: number
}

function LogsView({
  panelTitle,
  frames,
  fields,
  display,
  width,
  height,
}: ViewProps) {
  const [view, setView] = useState<ViewState>({
    order: display.order,
    wrap: display.wrap,
    showTime: display.showTime,
    dedup: display.dedup,
  })
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const sidebarId = useId()
  const change = useCallback(
    (next: Partial<ViewState>) =>
      setView((current) => ({ ...current, ...next })),
    []
  )
  const { entries, commonLabels, truncated } = useMemo(
    () =>
      logsModel(frames, fields, {
        order: view.order,
        dedup: view.dedup,
        prettify: display.prettify,
      }),
    [frames, fields, view.order, view.dedup, display.prettify]
  )
  const levelColumn = useMemo(
    () => entries.some((entry) => entry.level !== null),
    [entries]
  )
  const detailsMode: DetailsMode = !display.details
    ? "off"
    : display.detailsMode === "sidebar"
      ? "sidebar"
      : "inline"
  const selected = entries.find((entry) => entry.key === selectedKey)
  const commonText = labelText(commonLabels)

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
        <div className="min-h-0 min-w-0 flex-1 overflow-auto">
          <ul
            role="list"
            aria-label={`${panelTitle} log lines`}
            data-logs-count={entries.length}
            className="m-0 list-none p-0"
          >
            {entries.map((entry) => (
              <LogRow
                key={entry.key}
                entry={entry}
                fields={fields}
                wrap={view.wrap}
                showTime={view.showTime}
                showLabels={display.showLabels}
                highlight={display.highlight}
                levelColumn={levelColumn}
                detailsMode={detailsMode}
                selected={entry.key === selected?.key}
                sidebarId={sidebarId}
                onSelect={setSelectedKey}
              />
            ))}
          </ul>
        </div>
        {detailsMode === "sidebar" && selected ? (
          <aside
            aria-label="Log line details"
            className="min-w-48 shrink-0 basis-2/5 overflow-auto border-l border-border"
          >
            <LogDetails entry={selected} fields={fields} id={sidebarId} />
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
      display={display}
      width={width}
      height={height}
    />
  )
}

export default LogsPanel
