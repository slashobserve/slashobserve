import type { DatasourceStatus } from "@/datasources/queries"
import { traceUrlTemplate } from "@/lib/links"
import { traceDatasourceFor } from "@/logs/trace-link"
import type { FieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import {
  isPanelQuery,
  queryRef,
  resolveDatasourceRef,
  type DataPanel,
  type Override,
} from "@/spec"

const traceBackends = ["tempo", "jaeger", "clickhouse"]
const nanosecondBackends = ["tempo", "jaeger", "clickhouse"]
const durationColumn = "Duration"
const skipped = "skipped"

function isTraceIdColumn(name: string): boolean {
  return name.toLowerCase().replaceAll("_", "") === "traceid"
}

function datasourceRefOf(
  panel: DataPanel,
  frame: PreparedFrame
): string | undefined {
  const query = panel.queries.find(
    (candidate, at) => queryRef(candidate, at) === frame.ref
  )
  if (query !== undefined) {
    return isPanelQuery(query)
      ? (query.datasource ?? panel.datasource)
      : undefined
  }
  const refs = new Set(
    panel.queries
      .filter(isPanelQuery)
      .map((query) => query.datasource ?? panel.datasource)
  )
  return refs.size === 1 ? [...refs][0] : undefined
}

function sourceOf(
  panel: DataPanel,
  frame: PreparedFrame,
  all: readonly DatasourceStatus[]
): DatasourceStatus | undefined {
  const ref = datasourceRefOf(panel, frame)
  const uid = ref === undefined ? undefined : resolveDatasourceRef(ref, {})
  return all.find((datasource) => datasource.uid === uid)
}

function traceUidOf(
  source: DatasourceStatus,
  all: readonly DatasourceStatus[]
): string | null {
  return traceBackends.includes(source.type)
    ? source.uid
    : traceDatasourceFor(source, all)
}

type Choices = Map<string, Set<string>>

function note(choices: Choices, column: string, choice: string) {
  const seen = choices.get(column) ?? new Set<string>()
  seen.add(choice)
  choices.set(column, seen)
}

function agreed(choices: Choices): [string, string][] {
  return [...choices].flatMap(([column, seen]): [string, string][] => {
    const [only] = seen
    return seen.size === 1 && only !== undefined && only !== skipped
      ? [[column, only]]
      : []
  })
}

export function traceColumnOverrides(
  panel: DataPanel,
  frames: readonly PreparedFrame[],
  all: readonly DatasourceStatus[],
  fields: FieldResolver
): Override[] {
  if (panel.type !== "table") return []
  const links: Choices = new Map()
  const units: Choices = new Map()
  for (const frame of frames) {
    const source = sourceOf(panel, frame, all)
    for (const field of frame.fields) {
      const traceId = field.type === "string" && isTraceIdColumn(field.name)
      const duration = field.type === "number" && field.name === durationColumn
      if (!traceId && !duration) continue
      const config = fields.configFor(frame, field)
      if (traceId) {
        const uid = source ? traceUidOf(source, all) : null
        const open = uid !== null && config.links.length === 0
        note(links, field.name, open ? uid : skipped)
      } else {
        const open =
          source !== undefined &&
          nanosecondBackends.includes(source.type) &&
          config.unit === undefined
        note(units, field.name, open ? "ns" : skipped)
      }
    }
  }
  return [
    ...agreed(links).map(([field, uid]) => ({
      match: { field },
      links: [{ title: "Open trace", url: traceUrlTemplate(uid) }],
    })),
    ...agreed(units).map(([field]) => ({
      match: { field },
      unit: "ns" as const,
    })),
  ]
}
