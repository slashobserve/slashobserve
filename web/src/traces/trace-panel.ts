import type { DataPanel } from "@/spec"

import type { TraceSearch } from "./search"
import {
  clickhouseLatestTraceSpans,
  clickhouseTraceSpans,
  traceForm,
  traceQLFor,
} from "./search-query"

type TracePanelInput = {
  id: string
  title: string
  datasource: string
  type: string
  query: string
  y: number
}

export function tracesPanelFor({
  id,
  title,
  datasource,
  type,
  query,
  y,
}: TracePanelInput): DataPanel {
  return {
    id,
    type: "traces",
    title,
    datasource,
    gridPos: { x: 0, y, w: 24, h: 12 },
    queries: [type === "tempo" ? { query } : { query, format: "table" }],
  }
}

export function tracePanelQuery(type: string, search: TraceSearch): string {
  return type === "tempo"
    ? (search.q ?? traceQLFor(traceForm(search)))
    : clickhouseLatestTraceSpans(traceForm(search))
}

export function traceByIdQuery(type: string, traceId: string): string {
  return type === "tempo" ? traceId : clickhouseTraceSpans(traceId)
}

export function tracePanelTitle(search: TraceSearch): string {
  const named = [search.service, search.span].filter(Boolean).join(" ")
  return named === "" ? "Traces" : `Traces: ${named}`
}

export function placedPanel(
  panel: DataPanel,
  slot: { id: string; title: string; y: number }
): DataPanel {
  return {
    ...panel,
    id: slot.id,
    title: slot.title,
    gridPos: { ...panel.gridPos, y: slot.y },
  }
}
