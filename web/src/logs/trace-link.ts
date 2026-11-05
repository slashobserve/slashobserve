import type { DatasourceStatus } from "@/datasources/queries"
import type { LogEntry } from "@/panels/logs/logs-model"

const traceIdText = /^[0-9a-fA-F]{16}$|^[0-9a-fA-F]{32}$/
const allZeros = /^0+$/

export function detailValue(detail: LogEntry["details"][number]): string {
  return "text" in detail
    ? detail.text
    : String(detail.field.values[detail.index] ?? "")
}

export function traceIdOf(entry: LogEntry): string | null {
  for (const detail of entry.details) {
    if (detail.name.toLowerCase().replaceAll("_", "") !== "traceid") continue
    const text = detailValue(detail).trim()
    if (traceIdText.test(text) && !allZeros.test(text))
      return text.toLowerCase()
  }
  return null
}

export function traceDatasourceFor(
  logs: DatasourceStatus,
  all: readonly DatasourceStatus[]
): string | null {
  if (logs.type === "clickhouse") return logs.uid
  if (logs.type === "loki") {
    return all.find((datasource) => datasource.type === "tempo")?.uid ?? null
  }
  return null
}

export function traceRangeAround(timeMs: number): { from: string; to: string } {
  return {
    from: String(timeMs - 3_600_000),
    to: String(timeMs + 3_600_000),
  }
}
