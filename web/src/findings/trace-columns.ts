import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import { traceDatasourceFor } from "@/logs/trace-link"
import { isPanelQuery, type DataPanel, type Override } from "@/spec"

const traceBackends = ["tempo", "jaeger", "clickhouse"]
const nanosecondBackends = ["tempo", "jaeger"]
const durationColumn = "Duration"

function isTraceIdColumn(name: string): boolean {
  return name.toLowerCase().replaceAll("_", "") === "traceid"
}

function stringColumns(
  frames: readonly WireFrame[],
  match: (name: string) => boolean
): string[] {
  const names = frames.flatMap((frame) =>
    frame.fields
      .filter((field) => field.type === "string" && match(field.name))
      .map((field) => field.name)
  )
  return [...new Set(names)]
}

function datasourceOf(
  panel: DataPanel,
  all: readonly DatasourceStatus[]
): DatasourceStatus | undefined {
  const first = panel.queries.find(isPanelQuery)
  const uid = panel.datasource ?? first?.datasource
  return all.find((datasource) => datasource.uid === uid)
}

export function traceColumnOverrides(
  panel: DataPanel,
  frames: readonly WireFrame[],
  all: readonly DatasourceStatus[]
): Override[] {
  if (panel.type !== "table") return []
  const source = datasourceOf(panel, all)
  if (!source) return []
  const overrides: Override[] = []
  const traceUid = traceBackends.includes(source.type)
    ? source.uid
    : traceDatasourceFor(source, all)
  if (traceUid !== null) {
    for (const field of stringColumns(frames, isTraceIdColumn)) {
      overrides.push({
        match: { field },
        links: [
          {
            title: "Open trace",
            url: `/t/\${__value.raw}?ds=${encodeURIComponent(traceUid)}`,
          },
        ],
      })
    }
  }
  const hasDuration = frames.some((frame) =>
    frame.fields.some(
      (field) => field.type === "number" && field.name === durationColumn
    )
  )
  if (hasDuration && nanosecondBackends.includes(source.type)) {
    overrides.push({ match: { field: durationColumn }, unit: "ns" })
  }
  return overrides
}

export function withTraceColumns(
  panel: DataPanel,
  frames: readonly WireFrame[],
  all: readonly DatasourceStatus[]
): DataPanel {
  const added = traceColumnOverrides(panel, frames, all)
  if (added.length === 0) return panel
  return { ...panel, overrides: [...added, ...(panel.overrides ?? [])] }
}
