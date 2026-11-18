import type { DataPanel } from "@/spec"

import type { MetricsMode } from "./search"

export type MetricsPanelInput = {
  id: string
  title: string
  datasource: string
  query: string
  mode: MetricsMode
  y: number
}

export function metricsPanelFor({
  id,
  title,
  datasource,
  query,
  mode,
  y,
}: MetricsPanelInput): DataPanel {
  const common = {
    id,
    title,
    datasource,
    gridPos: { x: 0, y, w: 24, h: 10 },
  }
  if (mode === "table") {
    return {
      ...common,
      type: "table",
      queries: [{ query, instant: true, format: "table" }],
    }
  }
  return {
    ...common,
    type: "timeseries",
    queries: [{ query }],
    display: {
      legend: "bottom",
      legendMode: "table",
      legendValues: ["mean", "max", "last"],
    },
  }
}
