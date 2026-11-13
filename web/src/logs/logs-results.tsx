import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react"
import { Link } from "@tanstack/react-router"

import { CopyButton } from "@/components/copy-button"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import { traceLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import type { LogEntry, LogsDisplay } from "@/panels/logs/logs-model"
import {
  LogsView,
  type ColumnSort,
  type LogSelection,
  type LogsViewModel,
  type NewSince,
} from "@/panels/logs/logs-panel"
import type { DataPanel } from "@/spec"

import { anchorNs, serviceNameOf } from "./context-query"
import { ColumnsHeader, FieldsMenu } from "./fields-menu"
import { defaultColumns, fieldNames, lineId } from "./line-fields"
import { usePrepared } from "./prepared"
import { traceIdOf, traceRangeAround } from "./trace-link"
import { useElementSize } from "./use-element-size"

type ExploreLogsProps = {
  frames: WireFrame[]
  datasourceUid: string
  title: string
  height: number
  display: Record<string, unknown>
  detailActions?: (selection: LogSelection) => React.ReactNode
  isMarked?: (entry: LogEntry) => boolean
  isNew?: (entry: LogEntry) => boolean
  newSince?: NewSince
  selectedKey?: string | null
  onSelectedChange?: (selection: LogSelection | null) => void
  detailsId?: string
  columns?: readonly string[]
  columnSort?: ColumnSort | null
  onModel?: (model: LogsViewModel) => void
}

export function ExploreLogs({
  frames,
  datasourceUid,
  title,
  height,
  display,
  detailActions,
  isMarked,
  isNew,
  newSince,
  selectedKey,
  onSelectedChange,
  detailsId,
  columns,
  columnSort,
  onModel,
}: ExploreLogsProps) {
  const panel = useMemo<DataPanel>(
    () => ({
      id: "logs",
      type: "logs",
      title: "Logs",
      datasource: datasourceUid,
      gridPos: { x: 0, y: 0, w: 24, h: 10 },
      queries: [{ query: "" }],
      display,
    }),
    [datasourceUid, display]
  )
  const prepared = usePrepared(panel, frames)
  const { ref, width } = useElementSize<HTMLDivElement>()
  return (
    <div ref={ref} className="min-w-0" style={{ height }}>
      {width > 0 ? (
        <LogsView
          panelTitle={title}
          frames={prepared.frames}
          fields={prepared.fields}
          zone={undefined}
          display={prepared.display as LogsDisplay}
          width={width}
          height={height}
          detailActions={detailActions}
          isMarked={isMarked}
          isNew={isNew}
          newSince={newSince}
          selectedKey={selectedKey}
          onSelectedChange={onSelectedChange}
          detailsId={detailsId}
          columns={columns}
          columnSort={columnSort}
          onModel={onModel}
        />
      ) : null}
    </div>
  )
}

const phoneQuery = "(max-width: 899px)"

function subscribePhone(onChange: () => void) {
  const media = window.matchMedia(phoneQuery)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

function usePhone(): boolean {
  return useSyncExternalStore(
    subscribePhone,
    () => window.matchMedia(phoneQuery).matches,
    () => false
  )
}

const resultsDisplay = {
  showTime: true,
  showLabels: true,
  details: true,
  detailsMode: "inline",
  controls: false,
}

export type ResultsView = {
  order: "newest" | "oldest"
  wrap: boolean
  prettify: boolean
  columnSort: ColumnSort | null
}

type ResultsModel = LogsViewModel & { frames: WireFrame[] }

const columnSortValue = "column"

function sortOptions(sort: ColumnSort | null) {
  return [
    { value: "newest", label: "Newest first" },
    { value: "oldest", label: "Oldest first" },
    ...(sort === null
      ? []
      : [
          {
            value: columnSortValue,
            label: `${sort.name} ${sort.direction === "asc" ? "A to Z" : "Z to A"}`,
          },
        ]),
  ]
}

function TogglePill({
  pressed,
  label,
  onToggle,
  children,
}: {
  pressed: boolean
  label?: string
  onToggle: () => void
  children: React.ReactNode
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="xs"
      aria-pressed={pressed}
      aria-label={label}
      className="aria-pressed:bg-selection"
      onClick={onToggle}
    >
      {children}
    </Button>
  )
}

export function contextBlocker(
  datasource: DatasourceStatus,
  entry: LogEntry
): string | undefined {
  if (anchorNs(entry) === null) return "This line has no time."
  if (datasource.type === "clickhouse" && serviceNameOf(entry) === null) {
    return "This line has no ServiceName."
  }
  return undefined
}

export function OpenTraceLink({
  traceId,
  traceDatasource,
  time,
  className,
  children,
}: {
  traceId: string
  traceDatasource: string
  time: number
  className?: string
  children: React.ReactNode
}) {
  return (
    <Link
      {...traceLink(traceId, traceDatasource, traceRangeAround(time))}
      aria-label={`Open trace ${traceId}`}
      className={className}
    >
      {children}
    </Link>
  )
}

export function LogsResults({
  frames,
  datasource,
  traceDatasource,
  height,
  selectedKey,
  onSelect,
  onContext,
  detailsId,
  view,
  onViewChange,
  columns,
  onColumnsChange,
  linkedId,
  onLinkedChange,
  listTitle = "Query results",
  orderLocked = false,
  onRendered,
  isMarked,
  isNew,
  newSince,
}: {
  frames: WireFrame[]
  datasource: DatasourceStatus
  traceDatasource: string | null
  height: number
  selectedKey: string | null
  onSelect: (selection: LogSelection | null) => void
  onContext: (selection: LogSelection) => void
  detailsId: string
  view: ResultsView
  onViewChange: (change: Partial<ResultsView>) => void
  columns: readonly string[]
  onColumnsChange: (names: string[]) => void
  linkedId: string | undefined
  onLinkedChange: (id: string | undefined) => void
  listTitle?: string
  orderLocked?: boolean
  onRendered?: () => void
  isMarked?: (entry: LogEntry) => boolean
  isNew?: (entry: LogEntry) => boolean
  newSince?: NewSince
}) {
  const phone = usePhone()
  const sortId = useId()
  const { ref: rowsRef, height: rowsHeight } = useElementSize<HTMLDivElement>()
  const [emitted, setEmitted] = useState<ResultsModel | null>(null)
  const model = emitted?.frames === frames ? emitted : null
  const handleModel = useCallback(
    (next: LogsViewModel) => {
      setEmitted({ frames, ...next })
      onRendered?.()
    },
    [frames, onRendered]
  )
  const names = useMemo(
    () => (model === null ? [] : fieldNames(model.entries)),
    [model]
  )
  const shownColumns = useMemo(
    () =>
      columns.length > 0 ? columns : defaultColumns(datasource.type, names),
    [columns, datasource.type, names]
  )
  const levelColumn = useMemo(
    () => model?.entries.some((entry) => entry.level !== null) ?? false,
    [model]
  )

  const linked = useMemo(
    () =>
      model === null || linkedId === undefined
        ? undefined
        : model.entries.find((entry) => lineId(entry) === linkedId),
    [model, linkedId]
  )
  const selectedId = useMemo(() => {
    const entry =
      selectedKey === null
        ? undefined
        : model?.entries.find((candidate) => candidate.key === selectedKey)
    return entry === undefined ? null : lineId(entry)
  }, [model, selectedKey])
  const linkedMissing =
    !phone && model !== null && linkedId !== undefined && linked === undefined
  const opened = useRef<string | undefined>(undefined)
  const hadSelection = useRef(false)

  useEffect(() => {
    if (linkedId === undefined) {
      opened.current = undefined
      return
    }
    if (phone || model === null || linked === undefined) return
    if (opened.current === linkedId) return
    opened.current = linkedId
    const selection = model.selectionFor(linked.key)
    if (selection !== null) onSelect(selection)
  }, [linkedId, phone, model, linked, onSelect])

  useEffect(() => {
    if (phone) return
    if (selectedKey === null) {
      if (!hadSelection.current) return
      hadSelection.current = false
      if (linkedId !== undefined) onLinkedChange(undefined)
      return
    }
    if (selectedId === null) return
    hadSelection.current = true
    if (selectedId !== linkedId) {
      opened.current = selectedId
      onLinkedChange(selectedId)
    }
  }, [phone, selectedKey, selectedId, linkedId, onLinkedChange])

  const display = useMemo(
    () => ({
      ...resultsDisplay,
      order: view.order,
      wrap: view.wrap,
      prettify: view.prettify,
    }),
    [view.order, view.wrap, view.prettify]
  )
  const detailActions = useCallback(
    (selection: LogSelection) => {
      const { entry } = selection
      const traceId = traceIdOf(entry)
      const noContext = contextBlocker(datasource, entry)
      return (
        <>
          <Button
            type="button"
            variant="outline"
            size="xs"
            disabled={noContext !== undefined}
            title={noContext}
            onClick={() => onContext(selection)}
          >
            Show context
          </Button>
          {traceId !== null &&
          traceDatasource !== null &&
          entry.time !== null ? (
            <OpenTraceLink
              traceId={traceId}
              traceDatasource={traceDatasource}
              time={entry.time}
              className={cn(buttonVariants({ variant: "outline", size: "xs" }))}
            >
              Open trace
            </OpenTraceLink>
          ) : null}
          <CopyButton
            text={entry.message}
            label="Copy line"
            variant="outline"
            size="xs"
            icon={false}
          >
            Copy line
          </CopyButton>
        </>
      )
    },
    [datasource, onContext, traceDatasource]
  )
  const options = sortOptions(view.columnSort)
  return (
    <div className="flex flex-col" style={{ height }}>
      <div
        role="toolbar"
        aria-label="Results view"
        className="flex h-8 shrink-0 items-center gap-3 px-2 text-[13px]"
      >
        <div className="flex items-center gap-2">
          <Label htmlFor={sortId} className="text-[13px] font-normal">
            Sort
          </Label>
          <Select
            disabled={orderLocked}
            items={options}
            value={view.columnSort === null ? view.order : columnSortValue}
            onValueChange={(next) => {
              if (next === "newest" || next === "oldest") {
                onViewChange({ order: next, columnSort: null })
              }
            }}
          >
            <SelectTrigger id={sortId}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false} align="start">
              {options.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <TogglePill
          pressed={view.wrap}
          onToggle={() => onViewChange({ wrap: !view.wrap })}
        >
          Wrap
        </TogglePill>
        <TogglePill
          pressed={view.prettify}
          label="Pretty-print JSON"
          onToggle={() => onViewChange({ prettify: !view.prettify })}
        >
          JSON
        </TogglePill>
        {phone ? null : (
          <FieldsMenu
            names={names}
            chosen={shownColumns}
            onChange={onColumnsChange}
          />
        )}
        <span className="ml-auto text-xs text-muted-foreground max-[760px]:hidden">
          j and k move, Enter expands
        </span>
      </div>
      {linkedMissing ? (
        <div className="shrink-0 px-2 pb-2">
          <Alert>
            <AlertDescription className="flex items-center justify-between gap-2">
              The linked line is not in these results.
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={() => onLinkedChange(undefined)}
              >
                Clear
              </Button>
            </AlertDescription>
          </Alert>
        </div>
      ) : null}
      {phone ? null : (
        <ColumnsHeader
          columns={shownColumns}
          levelColumn={levelColumn}
          order={view.order}
          sort={view.columnSort}
          onFlipOrder={() => {
            if (orderLocked) return
            onViewChange({
              order: view.order === "newest" ? "oldest" : "newest",
              columnSort: null,
            })
          }}
          onSort={(columnSort) => {
            if (!orderLocked) onViewChange({ columnSort })
          }}
          onColumnsChange={onColumnsChange}
        />
      )}
      <div ref={rowsRef} className="min-h-0 flex-1">
        {rowsHeight > 0 ? (
          <ExploreLogs
            frames={frames}
            datasourceUid={datasource.uid}
            title={listTitle}
            height={rowsHeight}
            display={display}
            isMarked={isMarked}
            isNew={isNew}
            newSince={newSince}
            detailActions={phone ? detailActions : undefined}
            selectedKey={selectedKey}
            onSelectedChange={phone ? undefined : onSelect}
            detailsId={detailsId}
            columns={phone ? undefined : shownColumns}
            columnSort={view.columnSort}
            onModel={handleModel}
          />
        ) : null}
      </div>
    </div>
  )
}
