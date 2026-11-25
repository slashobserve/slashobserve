import { Link } from "@tanstack/react-router"
import { ChevronDownIcon } from "lucide-react"
import { useMemo, useState, type ReactNode } from "react"

import {
  barColumnMinWidth,
  barShare,
  textWidthCh,
} from "@/components/data-grid/bar"
import { BarCell } from "@/components/data-grid/bar-cell"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { traceLink } from "@/lib/links"
import { useObservationOrigin } from "@/observations/origin"
import { resolveTime, zonedDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"
import { styleForSlot } from "@/panels/colors"

import { DurationScatter } from "./duration-scatter"
import {
  fittedWindow,
  selectedRows,
  type Selection,
  type TimeWindow,
} from "./scatter"
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

const rowsP95Label = "p95 of these traces"

type Column = {
  key: TraceSortKey
  label: string
  align: "left" | "right"
  hideable: boolean
}

export const traceColumns: Column[] = [
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

function StartedLink({
  row,
  datasourceUid,
  range,
}: {
  row: TraceRow
  datasourceUid: string
  range: TraceRange
}) {
  const observation = useObservationOrigin()
  return (
    <Link
      {...traceLink(row.traceId, datasourceUid, range, undefined, observation)}
      aria-label={openTraceLabel(row)}
      title={row.start === null ? undefined : new Date(row.start).toISOString()}
      className="truncate font-medium tabular-nums underline-offset-3 hover:underline"
    >
      {row.start === null ? (
        <span className="font-mono">{row.traceId.slice(0, 8)}</span>
      ) : (
        zonedDateTimeMs(row.start, undefined)
      )}
    </Link>
  )
}

type TraceColumnContext = {
  slots: ReadonlyMap<string, number>
  slowest: number
  p95: number | null
  durationWidthCh: number
  datasourceUid: string
  range: TraceRange
}

export function traceGridColumn(
  column: Column,
  context: TraceColumnContext
): DataGridColumn<TraceRow> {
  const base = {
    id: column.key,
    header: column.label,
    align: column.align,
    sortable: true,
  }
  switch (column.key) {
    case "service":
      return {
        ...base,
        minWidth: 110,
        cell: (row) => (
          <span className="inline-flex min-w-0 items-center gap-1.5">
            <span
              aria-hidden="true"
              className="size-2 shrink-0 rounded-full"
              style={{
                backgroundColor: styleForSlot(
                  context.slots.get(row.service) ?? 0
                ).color,
              }}
            />
            <span className="truncate">{row.service || "-"}</span>
          </span>
        ),
      }
    case "span":
      return {
        ...base,
        minWidth: 110,
        flex: 1.2,
        cell: (row) => <span className="truncate">{row.span || "-"}</span>,
      }
    case "duration":
      return {
        ...base,
        minWidth: barColumnMinWidth(context.durationWidthCh),
        flex: 1.6,
        cell: (row) => (
          <BarCell
            text={durationText(row.durationMs)}
            widthCh={context.durationWidthCh}
            share={barShare(row.durationMs, context.slowest)}
            marker={
              context.p95 === null || context.slowest <= 0
                ? null
                : barShare(context.p95, context.slowest)
            }
            critical={row.errors !== null && row.errors > 0}
          />
        ),
      }
    case "spans":
      return {
        ...base,
        minWidth: 80,
        flex: 0.5,
        autoHeight: true,
        cell: (row) => (
          <span className="py-1 text-right leading-normal tabular-nums">
            {row.spans === null ? "-" : row.spans.toLocaleString("en")}
            {row.matching == null || row.matching === row.spans ? null : (
              <span className="block text-xs text-muted-foreground">
                {row.matching.toLocaleString("en")} matching
              </span>
            )}
          </span>
        ),
      }
    case "errors":
      return {
        ...base,
        minWidth: 130,
        flex: 1,
        cell: (row) =>
          row.errors === null ? "-" : <TraceStatus errors={row.errors} />,
      }
    case "start":
      return {
        ...base,
        minWidth: 130,
        flex: 1.4,
        cell: (row) => (
          <StartedLink
            row={row}
            datasourceUid={context.datasourceUid}
            range={context.range}
          />
        ),
      }
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
        render={
          <Button
            variant="ghost"
            size="sm"
            className="font-normal narrow:h-11"
          />
        }
      >
        Columns
        <ChevronDownIcon aria-hidden="true" className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {traceColumns
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
  fitWindow?: boolean
  headerActions?: ReactNode
}

export type TraceSortControl = {
  value: TraceSort | null
  onChange: (next: TraceSort | null) => void
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
  fitWindow = false,
  headerActions,
  selection,
  onSelectionChange,
  selectionActions,
  selectionPanel,
  sortControl,
}: {
  rows: TraceRow[]
  datasourceUid: string
  range: TraceRange
  hidden: ReadonlySet<TraceSortKey>
  onToggleColumn: (key: TraceSortKey, visible: boolean) => void
  sortControl?: TraceSortControl
} & TraceP95 &
  TraceSelection) {
  const [pageSort, setPageSort] = useState<TraceSort | null>(defaultTraceSort)
  const sort = sortControl === undefined ? pageSort : sortControl.value
  const setSort = sortControl === undefined ? setPageSort : sortControl.onChange
  const [highlighted, setHighlighted] = useState<string | null>(null)
  const listed = useMemo(
    () => (selection === null ? rows : selectedRows(rows, selection)),
    [rows, selection]
  )
  const sorted = useMemo(
    () => (sortControl === undefined ? sortTraceRows(listed, sort) : listed),
    [listed, sort, sortControl]
  )
  const window = useMemo(
    () =>
      (fitWindow ? fittedWindow(rows) : null) ??
      resolveWindow(range.from, range.to),
    [fitWindow, rows, range.from, range.to]
  )
  const slots = useMemo(() => serviceSlots(rows), [rows])

  const durations = rows.flatMap((row) =>
    row.durationMs === null ? [] : [row.durationMs]
  )
  const slowest = Math.max(0, ...durations)
  const p95 = p95Ms === undefined ? percentile(durations, 95) : p95Ms
  const spans = hasSpanCounts(rows)
  const status = hasErrorCounts(rows)
  const shownColumns = useMemo(
    () =>
      traceColumns.filter(
        (column) =>
          !hidden.has(column.key) &&
          (column.key !== "spans" || spans) &&
          (column.key !== "errors" || status)
      ),
    [hidden, spans, status]
  )
  const footer =
    footerNote ??
    (p95 === null
      ? null
      : `The tick on each bar is the ${durationText(p95)} ${p95Label}.`)
  const { from, to } = range
  const gridColumns = useMemo(() => {
    const context = {
      slots,
      slowest,
      p95,
      durationWidthCh: textWidthCh(
        rows.map((row) => durationText(row.durationMs))
      ),
      datasourceUid,
      range: { from, to },
    }
    return shownColumns.map((column) => traceGridColumn(column, context))
  }, [shownColumns, slots, slowest, p95, rows, datasourceUid, from, to])
  const plotted =
    window !== null &&
    rows.some((row) => row.start !== null && row.durationMs !== null)

  return (
    <div className="flex flex-col gap-3">
      <div
        className={cn(
          "grid items-start gap-4",
          selectionPanel !== undefined
            ? "grid-cols-[minmax(0,1fr)_384px] max-[1100px]:grid-cols-1"
            : "narrow:hidden"
        )}
      >
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
      </div>
      <div className="flex min-h-7 flex-wrap items-center gap-x-3 text-[13px]">
        <h2 className="font-semibold">
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
        <p className="ml-auto text-xs text-muted-foreground">
          {sortHint(sort)}
          {sortControl === undefined ? " Sorting applies to this page." : ""}
        </p>
        {headerActions}
        <ColumnsMenu hidden={hidden} onToggle={onToggleColumn} />
      </div>
      <DataGrid
        label="Traces"
        rows={sorted}
        columns={gridColumns}
        rowId={(row) => row.traceId}
        sort={sort}
        onSort={(key) => {
          const column = shownColumns.find((shown) => shown.key === key)
          if (column !== undefined) setSort(nextSort(sort, column.key))
        }}
        highlightedRowId={highlighted}
        onHighlightRow={setHighlighted}
        compact
      />
      {footer !== null ? (
        <p className="text-xs text-muted-foreground">{footer}</p>
      ) : null}
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
  sortControl?: TraceSortControl
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
