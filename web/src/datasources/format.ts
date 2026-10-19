import type { StatusTone } from "@/components/status-label"

import type { DatasourceStatus } from "./queries"

export function typeLabel(type: string): string {
  switch (type) {
    case "prometheus":
      return "Prometheus"
    case "clickhouse":
      return "ClickHouse, read only"
    default:
      return type
  }
}

export function usageLabel(count: number): string {
  if (count === 0) return "No dashboards"
  if (count === 1) return "1 dashboard"
  return `${count} dashboards`
}

export function statusOf(datasource: DatasourceStatus): {
  tone: StatusTone
  text: string
} {
  if (datasource.healthy) return { tone: "ok", text: "Healthy" }
  return {
    tone: "critical",
    text: datasource.message
      ? `Unavailable: ${datasource.message}`
      : "Unavailable",
  }
}
