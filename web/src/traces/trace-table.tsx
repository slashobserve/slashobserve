import { Link } from "@tanstack/react-router"
import { ChevronDownIcon, ChevronUpIcon, Columns3Icon } from "lucide-react"
import { useMemo, useState, type ReactNode } from "react"

import { StatusLabel } from "@/components/status-label"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { traceLink } from "@/lib/links"
import { resolveTime, zonedDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"
import { styleForSlot } from "@/panels/colors"

import { DurationScatter } from "./duration-scatter"
import { selectedRows, type Selection, type TimeWindow } from "./scatter"
import {
  countText,
  defaultTraceSort,
  durationText,
  hasErrorCounts,
  hasSpanCounts,
  nextSort,
  openTraceLabel,
  percentile,
  sortHint,
  sortTraceRows,
  type TraceRow,
  type TraceSort,
  type TraceSortKey,
} from "./trace-rows"

const pageSize = 20
const rowsP95Label = "p95 of these traces"

type Column = {
  key: TraceSortKey
  label: string
  align: "left" | "right"
  hideable: boolean
}

const columns: Column[] = [
  { key: "service", label: "Root service", align: "left", hideable: false },
  { key: "span", label: "Root span", align: "left", hideable: true },
  { key: "duration", label: "Duration", align: "left", hideable: false },
  { key: "spans", label: "Spans", align: "right", hideable: true },
  { key: "errors", label: "Errors", align: "left", hideable: true },
  { key: "start", label: "Started", align: "left", hideable: true },
]

type TraceRange = { from: string; to: string }

function TraceStatus({ errors }: { errors: number }) {
  return errors > 0 ? (
    <StatusLabel tone="critical">
      Failed · {countText(errors, "error")}
    </StatusLabel>
  ) : (
    <StatusLabel tone="ok">OK</StatusLabel>
  )
}

function SortHeader({
  column,
  sort,
  onSort,
}: {
  column: Column
  sort: TraceSort | null
  onSort: (key: TraceSortKey) => void
}) {
  const active = sort?.key === column.key ? sort : null
  const Arrow =
    active?.direction === "ascending" ? ChevronUpIcon : ChevronDownIcon
  return (
    <HeaderCell
      aria-sort={active?.direction ?? "none"}
      className={cn("p-0", column.align === "right" && "text-right")}
    >
      <button
        type="button"
        onClick={() => onSort(column.key)}
        className={cn(
          "flex w-full items-center gap-1 px-3.5 py-[9px] text-left hover:text-foreground",
          column.align === "right" && "flex-row-reverse text-right",
          active !== null && "text-foreground"
        )}
      >
        {column.label}
        {active !== null ? (
          <Arrow className="size-3" aria-hidden="true" />
        ) : null}
      </button>
    </HeaderCell>
  )
}

function DurationBar({
  row,
  slowest,
  p95,
}: {
  row: TraceRow
  slowest: number
  p95: number | null
}) {
  const scaled = (ms: number) => `${(ms / slowest) * 100}%`
  const failed = row.errors !== null && row.errors > 0
  return (
    <div className="flex items-center gap-2.5">
      <span className="w-18 shrink-0 text-right whitespace-nowrap tabular-nums">
        {durationText(row.durationMs)}
      </span>
      <div
        aria-hidden="true"
        className="relative h-1.5 min-w-20 flex-1 rounded-full bg-muted"
      >
        <div
          className={cn(
            "h-full rounded-full",
            failed ? "bg-status-critical" : "bg-series-1"
          )}
          style={{
            width:
              row.durationMs !== null && slowest > 0
                ? scaled(row.durationMs)
                : 0,
          }}
        />
        {p95 !== null && slowest > 0 ? (
          <div
            className="absolute inset-y-0 w-px -translate-x-1/2 bg-foreground"
            style={{ left: scaled(p95) }}
          />
        ) : null}
      </div>
    </div>
  )
}

function TraceCell({
  column,
  row,
  slot,
  slowest,
  p95,
  datasourceUid,
  range,
}: {
  column: Column
  row: TraceRow
  slot: number
  slowest: number
  p95: number | null
  datasourceUid: string
  range: TraceRange
}) {
  switch (column.key) {
    case "service":
      return (
        <Cell className="py-1">
          <span className="inline-flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: styleForSlot(slot).color }}
            />
            {row.service || "-"}
          </span>
        </Cell>
      )
    case "span":
      return <Cell className="max-w-80 truncate py-1">{row.span || "-"}</Cell>
    case "duration":
      return (
        <Cell className="py-1">
          <DurationBar row={row} slowest={slowest} p95={p95} />
        </Cell>
      )
    case "spans":
      return (
        <Cell className="py-1 text-right tabular-nums">
          {row.spans === null ? "-" : row.spans.toLocaleString("en")}
        </Cell>
      )
    case "errors":
      return (
        <Cell className="py-1 whitespace-nowrap">
          {row.errors === null ? "-" : <TraceStatus errors={row.errors} />}
        </Cell>
      )
    case "start":
      return (
        <Cell className="py-1 whitespace-nowrap">
          <Link
            {...traceLink(row.traceId, datasourceUid, range)}
            aria-label={openTraceLabel(row)}
            title={
              row.start === null ? undefined : new Date(row.start).toISOString()
            }
            className="font-medium tabular-nums underline-offset-3 hover:underline"
          >
            {row.start === null ? (
              <span className="font-mono">{row.traceId.slice(0, 8)}</span>
            ) : (
              zonedDateTimeMs(row.start, undefined)
            )}
          </Link>
        </Cell>
      )
  }
}

function resolveWindow(from: string, to: string): TimeWindow | null {
  const now = Date.now()
  const window = {
    start: resolveTime(from, now, false),
    end: resolveTime(to, now, true),
  }
  return Number.isFinite(window.start) && Number.isFinite(window.end)
    ? window
    : null
}

function serviceSlots(rows: readonly TraceRow[]): ReadonlyMap<string, number> {
  const services = [...new Set(rows.map((row) => row.service))].sort((a, b) =>
    a.localeCompare(b)
  )
  return new Map(services.map((service, index) => [service, index]))
}

function ColumnsMenu({
  hidden,
  onToggle,
}: {
  hidden: ReadonlySet<TraceSortKey>
  onToggle: (key: TraceSortKey, visible: boolean) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" size="xs" className="ml-auto" />}
      >
        <Columns3Icon aria-hidden="true" />
        Columns
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {columns
          .filter((column) => column.hideable)
          .map((column) => (
            <DropdownMenuCheckboxItem
              key={column.key}
              checked={!hidden.has(column.key)}
              onCheckedChange={(checked) => onToggle(column.key, checked)}
            >
              {column.label}
            </DropdownMenuCheckboxItem>
          ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

type TraceP95 = {
  p95Ms?: number | null
  p95Label?: string
  footerNote?: string
}

type TraceSelection = {
  selection: Selection | null
  onSelectionChange: (next: Selection | null) => void
  selectionActions?: ReactNode
  selectionPanel?: ReactNode
}

function SortedTraceTable({
  rows,
  datasourceUid,
  range,
  hidden,
  onToggleColumn,
  p95Ms,
  p95Label = rowsP95Label,
  footerNote,
  selection,
  onSelectionChange,
  selectionActions,
  selectionPanel,
}: {
  rows: TraceRow[]
  datasourceUid: string
  range: TraceRange
  hidden: ReadonlySet<TraceSortKey>
  onToggleColumn: (key: TraceSortKey, visible: boolean) => void
} & TraceP95 &
  TraceSelection) {
  const [sort, setSort] = useState<TraceSort | null>(defaultTraceSort)
  const [shown, setShown] = useState(pageSize)
  const [highlighted, setHighlighted] = useState<string | null>(null)
  const listed = useMemo(
    () => (selection === null ? rows : selectedRows(rows, selection)),
    [rows, selection]
  )
  const sorted = useMemo(() => sortTraceRows(listed, sort), [listed, sort])
  const window = useMemo(
    () => resolveWindow(range.from, range.to),
    [range.from, range.to]
  )
  const slots = useMemo(() => serviceSlots(rows), [rows])

  const durations = rows.flatMap((row) =>
    row.durationMs === null ? [] : [row.durationMs]
  )
  const slowest = Math.max(0, ...durations)
  const p95 = p95Ms === undefined ? percentile(durations, 95) : p95Ms
  const spans = hasSpanCounts(rows)
  const status = hasErrorCounts(rows)
  const shownColumns = columns.filter(
    (column) =>
      !hidden.has(column.key) &&
      (column.key !== "spans" || spans) &&
      (column.key !== "errors" || status)
  )
  const visible = sorted.slice(0, shown)
  const plotted =
    window !== null &&
    rows.some((row) => row.start !== null && row.durationMs !== null)

  return (
    <div className="flex flex-col gap-2">
      {plotted ? (
        <DurationScatter
          rows={rows}
          range={range}
          window={window}
          datasourceUid={datasourceUid}
          highlighted={highlighted}
          onHighlight={setHighlighted}
          p95Ms={p95}
          p95Label={p95Label}
          ariaLabel={
            p95Ms !== undefined && p95 !== null
              ? `Duration of matching traces; the ${p95Label} is ${durationText(p95)}`
              : undefined
          }
          selection={selection}
          onSelectionChange={onSelectionChange}
          selectedCount={listed.length}
          selectionActions={selectionActions}
        />
      ) : null}
      {selectionPanel}
      <div className="flex h-8 flex-wrap items-center gap-3 text-[13px]">
        <h2 className="font-medium">
          {selection === null
            ? countText(rows.length, "trace")
            : countText(listed.length, "selected trace")}
        </h2>
        {selection !== null ? (
          <p className="text-muted-foreground">
            Showing only the selection. Clear it to see all{" "}
            {rows.length.toLocaleString("en")}.
          </p>
        ) : null}
        <p className="text-muted-foreground">{sortHint(sort)}</p>
        <ColumnsMenu hidden={hidden} onToggle={onToggleColumn} />
      </div>
      <TableCard label="Traces" stackOnNarrow minWidth={760}>
        <thead>
          <tr>
            {shownColumns.map((column) => (
              <SortHeader
                key={column.key}
                column={column}
                sort={sort}
                onSort={(key) => setSort(nextSort(sort, key))}
              />
            ))}
          </tr>
        </thead>
        <tbody onPointerLeave={() => setHighlighted(null)}>
          {visible.map((row) => (
            <tr
              key={row.traceId}
              onPointerEnter={() => setHighlighted(row.traceId)}
              className={cn(
                "h-8 hover:bg-muted/40 narrow:h-auto",
                highlighted === row.traceId && "bg-selection"
              )}
            >
              {shownColumns.map((column) => (
                <TraceCell
                  key={column.key}
                  column={column}
                  row={row}
                  slot={slots.get(row.service) ?? 0}
                  slowest={slowest}
                  p95={p95}
                  datasourceUid={datasourceUid}
                  range={range}
                />
              ))}
            </tr>
          ))}
        </tbody>
      </TableCard>
      <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
        <p>
          {Math.min(shown, listed.length)} of {listed.length} traces shown.
          {footerNote !== undefined
            ? ` ${footerNote}`
            : p95 !== null
              ? ` The tick on each bar is the ${durationText(p95)} ${p95Label}.`
              : null}
        </p>
        {shown < listed.length ? (
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() => setShown((count) => count + pageSize)}
          >
            Show {pageSize} more
          </Button>
        ) : null}
      </div>
    </div>
  )
}

export function TraceTable({
  rows,
  datasourceUid,
  range,
  ...rest
}: {
  rows: TraceRow[]
  datasourceUid: string
  range: TraceRange
} & TraceP95 &
  TraceSelection) {
  const [hidden, setHidden] = useState<ReadonlySet<TraceSortKey>>(
    () => new Set()
  )

  function toggleColumn(key: TraceSortKey, visible: boolean) {
    setHidden((current) => {
      const next = new Set(current)
      if (visible) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <SortedTraceTable
      key={`${rows.length}:${rows[0]?.traceId ?? ""}`}
      rows={rows}
      datasourceUid={datasourceUid}
      range={range}
      hidden={hidden}
      onToggleColumn={toggleColumn}
      {...rest}
    />
  )
}
