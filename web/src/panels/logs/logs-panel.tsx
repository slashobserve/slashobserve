import { memo, useId, useMemo, useState } from "react"

import { formatDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"

import type { FieldResolver } from "../field-config"
import { thresholdIcons } from "../thresholds"
import type { PanelRenderer } from "../types"
import {
  detailText,
  logsModel,
  maxEntries,
  type LogEntry,
  type LogLevelKind,
  type LogsDisplay,
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

type RowProps = {
  entry: LogEntry
  fields: FieldResolver
  wrap: boolean
  showTime: boolean
  levelColumn: boolean
}

const LogRow = memo(function LogRow({
  entry,
  fields,
  wrap,
  showTime,
  levelColumn,
}: RowProps) {
  const [open, setOpen] = useState(false)
  const detailsId = useId()
  const kind = entry.level?.kind ?? "other"
  const Icon = kind === "other" ? null : levelIcons[kind]
  return (
    <li
      data-level={kind}
      className="border-b border-border [contain-intrinsic-size:auto_28px] [content-visibility:auto]"
    >
      <button
        type="button"
        aria-expanded={open}
        aria-controls={open ? detailsId : undefined}
        onClick={() => setOpen((current) => !current)}
        className="flex w-full items-start gap-2 px-2 py-1 text-left text-xs outline-none hover:bg-selection focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset aria-expanded:bg-selection"
      >
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
            wrap ? "[overflow-wrap:anywhere] whitespace-pre-wrap" : "truncate"
          )}
        >
          {entry.message}
        </span>
      </button>
      {open ? (
        <dl
          id={detailsId}
          className="grid grid-cols-[minmax(5rem,max-content)_minmax(0,1fr)] gap-x-3 gap-y-0.5 px-2 pt-0.5 pb-2 text-xs"
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
      ) : null}
    </li>
  )
})

export const LogsPanel: PanelRenderer = ({
  panel,
  frames,
  fields,
  display: rawDisplay,
  width,
  height,
}) => {
  const display = rawDisplay as LogsDisplay
  const { entries, truncated } = useMemo(
    () => logsModel(frames, fields, display),
    [frames, fields, display]
  )
  const levelColumn = useMemo(
    () => entries.some((entry) => entry.level !== null),
    [entries]
  )

  return (
    <div className="flex flex-col" style={{ width, height }}>
      {truncated ? (
        <p className="shrink-0 border-b border-border px-2 py-1.5 text-xs text-muted-foreground">
          Showing the {display.order === "oldest" ? "oldest" : "newest"}{" "}
          {maxEntries.toLocaleString("en")} lines.
        </p>
      ) : null}
      <div className="min-h-0 flex-1 overflow-auto">
        <ul
          role="list"
          aria-label={`${panel.title} log lines`}
          data-logs-count={entries.length}
          className="m-0 list-none p-0"
        >
          {entries.map((entry) => (
            <LogRow
              key={entry.key}
              entry={entry}
              fields={fields}
              wrap={display.wrap}
              showTime={display.showTime}
              levelColumn={levelColumn}
            />
          ))}
        </ul>
      </div>
    </div>
  )
}

export default LogsPanel
