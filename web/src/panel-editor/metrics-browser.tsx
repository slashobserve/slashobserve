import { useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { useDebouncedValue } from "@/lib/use-debounced-value"
import type { components } from "@/api/schema.gen"
import { Button } from "@/components/ui/button"

import { matcherInsert, metricInsert } from "./insert-text"
import {
  ListState,
  listClass,
  NameRow,
  nameSearchDelayMs,
  SearchBox,
  TruncatedNote,
} from "./name-list"
import {
  labelNamesQuery,
  labelValuesQuery,
  metricNamesQuery,
  type NameScope,
} from "./queries"

export type MetricsBrowserProps = {
  datasourceUid: string
  datasourceType: string | undefined
  onInsert: (text: string) => void
}

type MetricName = components["schemas"]["MetricName"]

type Selection =
  | { kind: "metric"; name: string; table?: string | undefined }
  | { kind: "table"; table: string }

type Entry = {
  id: string
  text: string
  title?: string | undefined
  note?: string | undefined
  selection: Selection
}

type NameList = {
  items: string[] | undefined
  truncated: boolean
  isError: boolean
  refetch: () => void
}

const clickhouseTables = ["otel_logs", "otel_traces"]

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

function selectionScope(selection: Selection | null): NameScope | null {
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

function BrowserBody({
  datasourceUid,
  datasourceType,
  onInsert,
}: MetricsBrowserProps) {
  const [metricsText, setMetricsText] = useState("")
  const [rightText, setRightText] = useState("")
  const [selected, setSelected] = useState<Selection | null>(null)
  const [label, setLabel] = useState<string | null>(null)

  const metricsSearch = useDebouncedValue(
    metricsText,
    metricsText,
    nameSearchDelayMs
  )
  const rightSearch = useDebouncedValue(rightText, rightText, nameSearchDelayMs)

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
      <SearchBox
        label={`Search ${searchTarget}`}
        value={narrowing ? rightText : metricsText}
        onChange={narrowing ? setRightText : setMetricsText}
      />
      <div className="grid grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] text-xs">
        <div className="border-r border-border">
          <ul aria-label="Metrics" className={listClass}>
            <ListState
              items={metrics.data ? entries : undefined}
              isError={metrics.isError}
              onRetry={() => void metrics.refetch()}
              empty="No metrics found."
            />
            {entries.map((entry) => (
              <NameRow
                key={entry.id}
                text={entry.text}
                onClick={() => select(entry.selection)}
                pressed={entry.id === selectedId}
                title={entry.title}
                note={entry.note}
              />
            ))}
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
                {right.items?.map((item) => (
                  <NameRow
                    key={item}
                    text={item}
                    onClick={() =>
                      label === null
                        ? openLabel(item)
                        : onInsert(matcherInsert(datasourceType, label, item))
                    }
                  />
                ))}
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
