import type { PanelDataResponse } from "@/dashboards/queries"
import { zonedDateTime } from "@/lib/time"
import {
  isDashboardQuery,
  isExpressionQuery,
  queryRef,
  resolveDatasourceRef,
  type DataPanel,
  type DataQuery,
  type Variable,
} from "@/spec"
import type { Selections } from "@/api/keys"

import { WaitingForData } from "./data-tab"
import type { InspectTarget } from "./inspect-pane"
import { queryVariables } from "./stats"

function savedText(query: DataQuery): string {
  if (isDashboardQuery(query)) return `Reuses panel ${query.source.panel}`
  if (isExpressionQuery(query)) {
    return query.expression.type === "math"
      ? query.expression.math
      : JSON.stringify(query.expression)
  }
  return query.query
}

function datasourceOf(
  panel: DataPanel,
  query: DataQuery,
  selections: Selections
): string {
  if (isExpressionQuery(query)) return "Server-side expression"
  if (isDashboardQuery(query)) return "Another panel's results"
  const ref = query.datasource ?? panel.datasource
  if (ref === undefined) return "Default"
  return resolveDatasourceRef(ref, selections) ?? ref
}

function stepText(stepMs: number): string {
  if (stepMs === 0) return "Instant"
  return stepMs % 1000 === 0 ? `${stepMs / 1000}s` : `${stepMs}ms`
}

function Fact({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{term}</dt>
      <dd className="min-w-0 wrap-anywhere">{children}</dd>
    </>
  )
}

function ResponseRange({
  meta,
  zone,
}: {
  meta: PanelDataResponse["meta"]
  zone: string | undefined
}) {
  return (
    <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-[13px]">
      <Fact term="From">{zonedDateTime(meta.from, zone)}</Fact>
      <Fact term="To">{zonedDateTime(meta.to, zone)}</Fact>
      <Fact term="Step">{stepText(meta.step_ms)}</Fact>
    </dl>
  )
}

export function QueryTab({
  target,
  variables,
  zone,
}: {
  target: InspectTarget | null
  variables: readonly Variable[]
  zone: string | undefined
}) {
  if (!target) return <WaitingForData />
  const { panel, data, selections } = target
  const names = variables.map((variable) => variable.name)
  return (
    <div className="flex flex-col gap-4">
      {data ? <ResponseRange meta={data.meta} zone={zone} /> : null}
      {panel.queries.map((query, index) => {
        const ref = queryRef(query, index)
        const text = savedText(query)
        const ran = data?.meta.queries.find((entry) => entry.ref === ref)
        const used = queryVariables(text, names)
        return (
          <section
            key={ref}
            aria-label={`Query ${ref}`}
            className="flex flex-col gap-2 rounded-card border p-3"
          >
            <h3 className="text-[13px] font-semibold">Query {ref}</h3>
            <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-[13px]">
              <Fact term="Datasource">
                <span className="font-mono text-xs">
                  {datasourceOf(panel, query, selections)}
                </span>
              </Fact>
              <Fact term="Step">
                {query.hide
                  ? "Hidden, so it does not run"
                  : ran
                    ? stepText(ran.step_ms)
                    : "Not run yet"}
              </Fact>
            </dl>
            <pre className="overflow-x-auto rounded-control border p-2 font-mono text-xs whitespace-pre-wrap">
              {text}
            </pre>
            {used.length > 0 ? (
              <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-[13px]">
                {used.map((name) => (
                  <Fact key={name} term={`$${name}`}>
                    {(selections[name] ?? []).join(", ") || "No value"}
                  </Fact>
                ))}
              </dl>
            ) : null}
          </section>
        )
      })}
    </div>
  )
}
