import type { DataPanel } from "@/spec"

export function logsPanelFor({
  id,
  title,
  datasource,
  query,
  y,
}: {
  id: string
  title: string
  datasource: string
  query: string
  y: number
}): DataPanel {
  return {
    id,
    type: "logs",
    title,
    datasource,
    gridPos: { x: 0, y, w: 24, h: 10 },
    queries: [{ query }],
    display: { showTime: true },
  }
}
