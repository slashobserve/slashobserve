import { useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { useDebouncedValue } from "@/lib/use-debounced-value"
import type { NameListParams } from "@/api/keys"
import {
  ListState,
  listClass,
  NameRow,
  nameSearchDelayMs,
  SearchBox,
  TruncatedNote,
} from "@/panel-editor/name-list"
import { labelNamesQuery, labelValuesQuery } from "@/panel-editor/queries"

type Props = {
  datasourceUid: string
  datasourceType: string
  onPick: (label: string, value: string) => void
}

function BrowserBody({ datasourceUid, datasourceType, onPick }: Props) {
  const [labelText, setLabelText] = useState("")
  const [valueText, setValueText] = useState("")
  const [label, setLabel] = useState<string | null>(null)
  const labelSearch = useDebouncedValue(labelText, labelText, nameSearchDelayMs)
  const valueSearch = useDebouncedValue(valueText, valueText, nameSearchDelayMs)
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
              <NameRow
                key={name}
                text={name}
                pressed={name === label}
                onClick={() => {
                  setLabel(name)
                  setValueText("")
                }}
              />
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
                  <NameRow
                    key={value}
                    text={value}
                    onClick={() => onPick(label, value)}
                  />
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
