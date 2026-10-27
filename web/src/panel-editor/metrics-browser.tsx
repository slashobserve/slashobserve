import { useQuery } from "@tanstack/react-query"
import { SearchIcon } from "lucide-react"
import { useEffect, useState } from "react"

import type { NameListParams } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import { matcherInsert, metricInsert } from "./insert-text"
import { labelNamesQuery, labelValuesQuery, metricNamesQuery } from "./queries"

export type MetricsBrowserProps = {
  datasourceUid: string
  datasourceType: string | undefined
  onInsert: (text: string) => void
}

type MetricName = components["schemas"]["MetricName"]

type Selection =
  | { kind: "metric"; name: string; table?: string }
  | { kind: "table"; table: string }

type Entry = {
  id: string
  text: string
  title?: string
  note?: string
  selection: Selection
}

type NameList = {
  items: string[] | undefined
  truncated: boolean
  isError: boolean
  refetch: () => void
}

const searchDelayMs = 250
const skeletonRows = [0, 1, 2]
const clickhouseTables = ["otel_logs", "otel_traces"]
const listClass = "m-0 max-h-64 list-none overflow-y-auto py-1"
const rowClass =
  "block w-full truncate px-2.5 py-[5px] text-left font-mono hover:bg-selection focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"

function useDebounced<T>(value: T): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), searchDelayMs)
    return () => clearTimeout(timer)
  }, [value])
  return debounced
}

function metricEntries(
  type: string | undefined,
  metrics: MetricName[],
  search: string
): Entry[] {
  const entries: Entry[] = metrics.map((metric) => ({
    id: `metric:${metric.table ?? ""}:${metric.name}`,
    text: metric.name,
    title: metric.help,
    note: metric.table,
    selection: { kind: "metric", name: metric.name, table: metric.table },
  }))
  if (type !== "clickhouse") return entries
  const needle = search.toLowerCase()
  for (const table of clickhouseTables) {
    if (!table.includes(needle)) continue
    entries.push({
      id: `table:${table}`,
      text: `${table} (table)`,
      selection: { kind: "table", table },
    })
  }
  return entries
}

function selectionScope(selection: Selection | null): NameListParams | null {
  if (selection === null) return null
  if (selection.kind === "table") return { table: selection.table }
  return { metric: selection.name, table: selection.table }
}

function selectionId(selection: Selection | null): string | null {
  if (selection === null) return null
  if (selection.kind === "table") return `table:${selection.table}`
  return `metric:${selection.table ?? ""}:${selection.name}`
}

function insertTextFor(type: string | undefined, selection: Selection) {
  return selection.kind === "table"
    ? selection.table
    : metricInsert(type, selection.name)
}

function ListState({
  items,
  isError,
  onRetry,
  empty,
}: {
  items: unknown[] | undefined
  isError: boolean
  onRetry: () => void
  empty: string
}) {
  if (isError) {
    return (
      <li className="flex flex-col items-start gap-1.5 px-2.5 py-1.5">
        <span className="text-muted-foreground">Could not load names.</span>
        <Button type="button" variant="outline" size="xs" onClick={onRetry}>
          Retry
        </Button>
      </li>
    )
  }
  if (items === undefined) {
    return skeletonRows.map((row) => (
      <li key={row} className="px-2.5 py-[5px]">
        <Skeleton className="h-[18px]" />
      </li>
    ))
  }
  if (items.length === 0) {
    return <li className="px-2.5 py-[5px] text-muted-foreground">{empty}</li>
  }
  return null
}

function TruncatedNote({ show }: { show: boolean }) {
  if (!show) return null
  return (
    <p className="m-0 border-t border-border px-2.5 py-1.5 text-muted-foreground">
      Showing the first 100. Type to narrow.
    </p>
  )
}

function rowButton(
  key: string,
  text: string,
  onClick: () => void,
  options: { pressed?: boolean; title?: string; note?: string } = {}
) {
  return (
    <li key={key}>
      <button
        type="button"
        className={cn(rowClass, options.pressed && "bg-selection")}
        aria-pressed={options.pressed}
        title={options.title}
        onClick={onClick}
      >
        {text}
        {options.note ? (
          <span className="ml-2 font-sans text-muted-foreground">
            {options.note}
          </span>
        ) : null}
      </button>
    </li>
  )
}

function BrowserBody({
  datasourceUid,
  datasourceType,
  onInsert,
}: MetricsBrowserProps) {
  const [metricsText, setMetricsText] = useState("")
  const [rightText, setRightText] = useState("")
  const [selected, setSelected] = useState<Selection | null>(null)
  const [label, setLabel] = useState<string | null>(null)

  const metricsSearch = useDebounced(metricsText)
  const rightSearch = useDebounced(rightText)

  const metrics = useQuery(metricNamesQuery(datasourceUid, metricsSearch))
  const scope = selectionScope(selected)
  const enabled = scope !== null
  const rootScope = scope ?? {}

  const labelsEnabled = enabled && label === null
  const valuesEnabled = enabled && label !== null
  const labelsBase = useQuery({
    ...labelNamesQuery(datasourceUid, rootScope),
    enabled: labelsEnabled,
  })
  const labelsShown = useQuery({
    ...labelNamesQuery(datasourceUid, {
      ...rootScope,
      search: rightSearch || undefined,
    }),
    enabled: labelsEnabled,
  })
  const valuesBase = useQuery({
    ...labelValuesQuery(datasourceUid, label ?? "", rootScope),
    enabled: valuesEnabled,
  })
  const valuesShown = useQuery({
    ...labelValuesQuery(datasourceUid, label ?? "", {
      ...rootScope,
      search: rightSearch || undefined,
    }),
    enabled: valuesEnabled,
  })

  const right: NameList =
    label === null
      ? {
          items: labelsShown.data?.labels,
          truncated: labelsShown.data?.truncated ?? false,
          isError: labelsShown.isError,
          refetch: () => void labelsShown.refetch(),
        }
      : {
          items: valuesShown.data?.values,
          truncated: valuesShown.data?.truncated ?? false,
          isError: valuesShown.isError,
          refetch: () => void valuesShown.refetch(),
        }
  const rightTruncated =
    (label === null
      ? labelsBase.data?.truncated
      : valuesBase.data?.truncated) ?? false
  const narrowing = enabled && rightTruncated
  const searchTarget = narrowing
    ? label === null
      ? "labels"
      : "values"
    : "metrics"

  const entries = metricEntries(
    datasourceType,
    metrics.data?.metrics ?? [],
    metricsSearch
  )
  const selectedId = selectionId(selected)

  function select(selection: Selection) {
    setSelected(selection)
    setLabel(null)
    setRightText("")
  }

  function openLabel(name: string) {
    setLabel(name)
    setRightText("")
  }

  function backToLabels() {
    setLabel(null)
    setRightText("")
  }

  return (
    <section
      aria-label="Metrics and labels"
      className="rounded-lg border border-border"
    >
      <div className="flex items-center gap-2 border-b border-border px-2.5 py-2">
        <SearchIcon
          aria-hidden="true"
          className="size-[13px] shrink-0 text-muted-foreground"
        />
        <input
          aria-label={`Search ${searchTarget}`}
          className="min-w-0 flex-1 rounded-control border-0 bg-transparent font-mono text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring"
          value={narrowing ? rightText : metricsText}
          onChange={(event) =>
            narrowing
              ? setRightText(event.target.value)
              : setMetricsText(event.target.value)
          }
        />
      </div>
      <div className="grid grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] text-xs">
        <div className="border-r border-border">
          <ul aria-label="Metrics" className={listClass}>
            <ListState
              items={metrics.data ? entries : undefined}
              isError={metrics.isError}
              onRetry={() => void metrics.refetch()}
              empty="No metrics found."
            />
            {entries.map((entry) =>
              rowButton(entry.id, entry.text, () => select(entry.selection), {
                pressed: entry.id === selectedId,
                title: entry.title,
                note: entry.note,
              })
            )}
          </ul>
          <TruncatedNote show={metrics.data?.truncated ?? false} />
        </div>
        <div>
          {label !== null ? (
            <div className="px-2.5 pt-1.5">
              <Button
                type="button"
                variant="ghost"
                size="xs"
                className="px-0 text-muted-foreground hover:bg-transparent dark:hover:bg-transparent"
                onClick={backToLabels}
              >
                Back to labels
              </Button>
            </div>
          ) : null}
          {selected === null ? (
            <p className="m-0 px-2.5 py-2 text-muted-foreground">
              Select a metric to see its labels.
            </p>
          ) : (
            <>
              <ul
                aria-label={label === null ? "Labels" : "Values"}
                className={listClass}
              >
                <ListState
                  items={right.items}
                  isError={right.isError}
                  onRetry={right.refetch}
                  empty={
                    label === null ? "No labels found." : "No values found."
                  }
                />
                {right.items?.map((item) =>
                  rowButton(item, item, () =>
                    label === null
                      ? openLabel(item)
                      : onInsert(matcherInsert(datasourceType, label, item))
                  )
                )}
              </ul>
              <TruncatedNote show={right.truncated} />
            </>
          )}
        </div>
      </div>
      <div className="flex gap-2 border-t border-border px-2.5 py-2">
        <Button
          type="button"
          variant="outline"
          size="xs"
          disabled={selected === null}
          onClick={() => {
            if (selected !== null) {
              onInsert(insertTextFor(datasourceType, selected))
            }
          }}
        >
          {selected?.kind === "table" ? "Insert table" : "Insert metric"}
        </Button>
      </div>
    </section>
  )
}

export function MetricsBrowser(props: MetricsBrowserProps) {
  return <BrowserBody key={props.datasourceUid} {...props} />
}
