import type { StatusTone } from "@/components/status-label"

import type { DatasourceStatus } from "./queries"

export function typeLabel(type: string): string {
  switch (type) {
    case "prometheus":
      return "Prometheus"
    case "clickhouse":
      return "ClickHouse, read only"
    case "loki":
      return "Loki"
    case "tempo":
      return "Tempo"
    case "jaeger":
      return "Jaeger"
    case "elasticsearch":
      return "Elasticsearch"
    case "opensearch":
      return "OpenSearch"
    default:
      return type
  }
}

export type ExploreTarget = "logs" | "traces" | "metrics"

export function exploreTargets(type: string): ExploreTarget[] {
  switch (type) {
    case "clickhouse":
      return ["logs", "traces"]
    case "loki":
    case "elasticsearch":
    case "opensearch":
      return ["logs"]
    case "tempo":
    case "jaeger":
      return ["traces"]
    case "prometheus":
      return ["metrics"]
    default:
      return []
  }
}

export function sourceLabel(source: DatasourceStatus["source"]): string {
  switch (source) {
    case "file":
      return "From file"
    case "app":
      return "Added in the app"
    case "builtin":
      return "Built in"
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
