import { useQuery } from "@tanstack/react-query"
import { SearchIcon } from "lucide-react"
import { useEffect, useState } from "react"

import type { NameListParams } from "@/api/keys"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { matcherInsert } from "@/panel-editor/insert-text"
import { labelNamesQuery, labelValuesQuery } from "@/panel-editor/queries"
import { cn } from "@/lib/utils"

type Props = {
  datasourceUid: string
  datasourceType: string
  onInsert: (text: string) => void
}

const searchDelayMs = 250
const skeletonRows = [0, 1, 2]
const listClass = "m-0 max-h-64 list-none overflow-y-auto py-1"
const rowClass =
  "block w-full truncate px-2.5 py-[5px] text-left font-mono hover:bg-selection focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
const searchClass =
  "min-w-0 flex-1 rounded-control border-0 bg-transparent font-mono text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring"

function useDebounced<T>(value: T): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), searchDelayMs)
    return () => clearTimeout(timer)
  }, [value])
  return debounced
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

function SearchBox({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex items-center gap-2 border-b border-border px-2.5 py-2">
      <SearchIcon
        aria-hidden="true"
        className="size-[13px] shrink-0 text-muted-foreground"
      />
      <input
        aria-label={label}
        className={searchClass}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  )
}

function BrowserBody({ datasourceUid, datasourceType, onInsert }: Props) {
  const [labelText, setLabelText] = useState("")
  const [valueText, setValueText] = useState("")
  const [label, setLabel] = useState<string | null>(null)
  const labelSearch = useDebounced(labelText)
  const valueSearch = useDebounced(valueText)
  const scope: NameListParams =
    datasourceType === "clickhouse" ? { table: "otel_logs" } : {}

  const labels = useQuery(
    labelNamesQuery(datasourceUid, {
      ...scope,
      search: labelSearch || undefined,
    })
  )
  const values = useQuery({
    ...labelValuesQuery(datasourceUid, label ?? "", {
      ...scope,
      search: valueSearch || undefined,
    }),
    enabled: label !== null,
  })

  return (
    <section
      aria-label="Labels"
      className="rounded-card border border-border text-xs"
    >
      <div className="grid grid-cols-2">
        <div className="border-r border-border">
          <SearchBox
            label="Search labels"
            value={labelText}
            onChange={setLabelText}
          />
          <ul aria-label="Label names" className={listClass}>
            <ListState
              items={labels.data?.labels}
              isError={labels.isError}
              onRetry={() => void labels.refetch()}
              empty="No labels found."
            />
            {labels.data?.labels.map((name) => (
              <li key={name}>
                <button
                  type="button"
                  className={cn(rowClass, name === label && "bg-selection")}
                  aria-pressed={name === label}
                  onClick={() => {
                    setLabel(name)
                    setValueText("")
                  }}
                >
                  {name}
                </button>
              </li>
            ))}
          </ul>
          <TruncatedNote show={labels.data?.truncated ?? false} />
        </div>
        <div>
          {label === null ? (
            <p className="m-0 px-2.5 py-2 text-muted-foreground">
              Select a label to see its values.
            </p>
          ) : (
            <>
              <SearchBox
                label="Search values"
                value={valueText}
                onChange={setValueText}
              />
              <ul aria-label="Label values" className={listClass}>
                <ListState
                  items={values.data?.values}
                  isError={values.isError}
                  onRetry={() => void values.refetch()}
                  empty="No values found."
                />
                {values.data?.values.map((value) => (
                  <li key={value}>
                    <button
                      type="button"
                      className={rowClass}
                      onClick={() =>
                        onInsert(matcherInsert(datasourceType, label, value))
                      }
                    >
                      {value}
                    </button>
                  </li>
                ))}
              </ul>
              <TruncatedNote show={values.data?.truncated ?? false} />
            </>
          )}
        </div>
      </div>
    </section>
  )
}

export function LogLabelBrowser(props: Props) {
  return <BrowserBody key={props.datasourceUid} {...props} />
}
