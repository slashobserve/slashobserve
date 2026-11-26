import { useCallback, useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { EmptyState } from "@/components/empty-state"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { cn } from "@/lib/utils"
import { Sparkline } from "@/panels/stat/sparkline"
import { sparklinePoints } from "@/panels/stat/sparkline-points"

import {
  defaultPatternLimit,
  logPatternsQuery,
  maxPatternLimit,
  patternCountText,
  patternParts,
  patternShare,
  patternsErrorText,
  patternsFooter,
  patternsHeading,
  patternsNote,
  shareText,
  smallPatterns,
  sortPatterns,
  trendChange,
  withPatternFilter,
  type LogPattern,
  type PatternMode,
  type PatternOrder,
} from "./patterns"
import { logVolumeQuery } from "./volume"
import { useLogsWorkspace } from "./workspace"

const orders: { value: PatternOrder; label: string }[] = [
  { value: "count", label: "Most lines first" },
  { value: "newest", label: "Newest first" },
]
const skeletonRows = [0, 1, 2, 3, 4]
const levelClasses: Record<LogPattern["level"], string> = {
  error: "bg-status-critical",
  warning: "bg-status-warning",
  info: "bg-series-1",
  other: "bg-muted-foreground/40",
  "": "bg-transparent",
}

function useLogPatterns(enabled: boolean, limit: number) {
  const { datasource, effectiveQuery, range } = useLogsWorkspace()
  return useQuery({
    ...logPatternsQuery(datasource.uid, {
      query: effectiveQuery ?? "",
      from: range.from,
      to: range.to,
      limit,
    }),
    enabled: enabled && effectiveQuery !== undefined,
  })
}

export function PatternsTabLabel() {
  const cached = useLogPatterns(false, defaultPatternLimit)
  return (
    <>
      Patterns
      {cached.data === undefined ? null : (
        <span className="text-xs text-muted-foreground tabular-nums">
          {patternCountText(cached.data)}
        </span>
      )}
    </>
  )
}

function PatternText({ pattern }: { pattern: string }) {
  return (
    <span className="min-w-0 truncate font-mono text-[12.5px]" title={pattern}>
      {patternParts(pattern).map((part, index) => (
        <span
          key={index}
          className={part.wildcard ? "text-muted-foreground" : undefined}
        >
          {part.text}
        </span>
      ))}
    </span>
  )
}

function FilterButton({
  label,
  ariaLabel,
  query,
  onApply,
}: {
  label: string
  ariaLabel: string
  query: string | null
  onApply: (query: string) => void
}) {
  return (
    <span title={query === null ? "Cannot edit this query" : undefined}>
      <Button
        type="button"
        variant="ghost"
        size="xs"
        aria-label={ariaLabel}
        disabled={query === null}
        onClick={() => query !== null && onApply(query)}
      >
        {label}
      </Button>
    </span>
  )
}

type PatternRewrite = (pattern: string, mode: PatternMode) => string | null

export function patternColumns({
  total,
  largest,
  expanded,
  onToggle,
  rewrite,
  onApply,
}: {
  total: number
  largest: number
  expanded: ReadonlySet<string>
  onToggle: (pattern: string) => void
  rewrite: PatternRewrite
  onApply: (query: string) => void
}): DataGridColumn<LogPattern>[] {
  return [
    {
      id: "pattern",
      header: "Pattern",
      minWidth: 160,
      flex: 1,
      cell: (item) => (
        <div className="flex min-w-0 items-center gap-2">
          <span
            aria-hidden="true"
            className={cn(
              "size-2 shrink-0 rounded-control",
              levelClasses[item.level]
            )}
          />
          {item.level === "" ? null : (
            <span className="sr-only">{item.level}</span>
          )}
          <PatternText pattern={item.pattern} />
        </div>
      ),
    },
    {
      id: "documents",
      header: "Documents",
      align: "right",
      width: 110,
      cell: (item) => (
        <div className="flex flex-col items-end gap-1 tabular-nums">
          {item.count.toLocaleString("en")}
          <span
            aria-hidden="true"
            className="h-1 rounded-control bg-series-1/60"
            style={{
              width: `${largest === 0 ? 0 : Math.round((item.count / largest) * 100)}%`,
            }}
          />
        </div>
      ),
    },
    {
      id: "share",
      header: "Share",
      align: "right",
      width: 80,
      cell: (item) => (
        <span className="tabular-nums">
          {shareText(patternShare(item.count, total))}
        </span>
      ),
    },
    {
      id: "trend",
      header: "Trend",
      width: 170,
      cell: (item) => {
        const change = trendChange(item.trend)
        return (
          <div className="flex items-center gap-2">
            {item.trend.length === 0 ? null : (
              <span className="block h-6 w-24">
                <Sparkline
                  line={sparklinePoints(
                    item.trend.map((bucket, index) => [index, bucket.count]),
                    30
                  )}
                  stroke="var(--series-1)"
                  area={false}
                />
              </span>
            )}
            {change === null ? null : (
              <span className="text-xs whitespace-nowrap text-muted-foreground tabular-nums">
                {change}
              </span>
            )}
          </div>
        )
      },
    },
    {
      id: "actions",
      header: "Actions",
      align: "right",
      action: true,
      width: 240,
      cell: (item) => (
        <div className="flex flex-wrap items-center justify-end gap-1">
          <FilterButton
            label="Filter"
            ariaLabel="Filter to this pattern"
            query={rewrite(item.pattern, "in")}
            onApply={onApply}
          />
          <FilterButton
            label="Filter out"
            ariaLabel="Filter out this pattern"
            query={rewrite(item.pattern, "out")}
            onApply={onApply}
          />
          {item.sample === "" ? null : (
            <Button
              type="button"
              variant="ghost"
              size="xs"
              aria-expanded={expanded.has(item.pattern)}
              aria-label={`${expanded.has(item.pattern) ? "Hide" : "Show"} a sample line of this pattern`}
              onClick={() => onToggle(item.pattern)}
            >
              Samples
            </Button>
          )}
        </div>
      ),
    },
  ]
}

const patternRowId = (item: LogPattern) => item.pattern

function PatternsTable({
  items,
  total,
  largest,
  expanded,
  onToggle,
  rewrite,
  onApply,
}: {
  items: LogPattern[]
  total: number
  largest: number
  expanded: ReadonlySet<string>
  onToggle: (pattern: string) => void
  rewrite: PatternRewrite
  onApply: (query: string) => void
}) {
  const columns = useMemo(
    () =>
      patternColumns({ total, largest, expanded, onToggle, rewrite, onApply }),
    [total, largest, expanded, onToggle, rewrite, onApply]
  )
  const detail = useMemo(
    () => ({
      open: expanded,
      render: (item: LogPattern) => (
        <p className="m-0 font-mono text-xs break-all text-muted-foreground">
          {item.sample}
        </p>
      ),
    }),
    [expanded]
  )
  return (
    <DataGrid
      label="Patterns"
      rows={items}
      columns={columns}
      rowId={patternRowId}
      detail={detail}
    />
  )
}

function PatternsSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-2">
      {skeletonRows.map((row) => (
        <Skeleton key={row} className="h-6" />
      ))}
    </div>
  )
}

export function PatternsView() {
  const { datasource, search, update, setDraft, effectiveQuery, range } =
    useLogsWorkspace()
  const [limit, setLimit] = useState(defaultPatternLimit)
  const [order, setOrder] = useState<PatternOrder>("count")
  const [showAll, setShowAll] = useState(false)
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set())
  const toggle = useCallback((pattern: string) => {
    setExpanded((current) => {
      const next = new Set(current)
      if (!next.delete(pattern)) next.add(pattern)
      return next
    })
  }, [])
  const patterns = useLogPatterns(true, limit)
  const volume = useQuery({
    ...logVolumeQuery(datasource.uid, {
      query: effectiveQuery ?? "",
      from: range.from,
      to: range.to,
    }),
    enabled: false,
  })
  const q = search.q
  const source = patterns.data?.source ?? "server"
  const datasourceType = datasource.type
  const filterByPattern = useCallback(
    (query: string) => {
      setDraft(query)
      update({ q: query, view: undefined })
    },
    [setDraft, update]
  )
  const rewrite = useCallback(
    (pattern: string, mode: PatternMode) =>
      withPatternFilter(datasourceType, q ?? "", pattern, mode, source),
    [datasourceType, q, source]
  )

  if (q === undefined) {
    return <EmptyState title="Write a query and press Run" />
  }
  if (patterns.isError) {
    const { text, retry } = patternsErrorText(patterns.error)
    return (
      <div className="p-3">
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col items-start gap-2">
            {text}
            {retry ? (
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={() => void patterns.refetch()}
              >
                Retry
              </Button>
            ) : null}
          </AlertDescription>
        </Alert>
      </div>
    )
  }
  if (patterns.data === undefined) {
    return (
      <div className="p-3">
        <PatternsSkeleton />
      </div>
    )
  }
  const result = patterns.data
  if (result.patterns.length === 0) {
    return (
      <p className="p-3 text-[13px] text-muted-foreground">
        No patterns in this range.
      </p>
    )
  }

  const { shown, folded } = smallPatterns(result.patterns, result.lines_matched)
  const visible = sortPatterns(showAll ? result.patterns : shown, order)
  const largest = Math.max(0, ...result.patterns.map((item) => item.count))

  return (
    <div className="flex flex-col gap-3 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="m-0 text-[13px] text-muted-foreground">
          {patternsHeading(result, volume.data?.total)}
        </p>
        <Select
          items={orders}
          value={order}
          onValueChange={(next) => next && setOrder(next)}
        >
          <SelectTrigger aria-label="Sort patterns">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="end">
            {orders.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <PatternsTable
        items={visible}
        total={result.lines_matched}
        largest={largest}
        expanded={expanded}
        onToggle={toggle}
        rewrite={rewrite}
        onApply={filterByPattern}
      />
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        {folded.length > 0 && !showAll ? (
          <>
            <span>
              {patternsFooter(
                shown.length,
                result.patterns.length,
                folded.length
              )}
            </span>
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => setShowAll(true)}
            >
              Show all
            </Button>
          </>
        ) : null}
        {result.truncated && limit < maxPatternLimit ? (
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() => setLimit(maxPatternLimit)}
          >
            Show {maxPatternLimit}
          </Button>
        ) : null}
      </div>
      <p className="m-0 text-xs text-muted-foreground">{patternsNote}</p>
    </div>
  )
}
