import type { DataPanel } from "@/spec"

import { hasQueryText, type TraceSearch } from "./search"
import {
  clickhouseLatestTraceSpans,
  clickhouseTraceSpans,
  textSearchFor,
  traceForm,
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
    queries: [hasQueryText(type) ? { query } : { query, format: "table" }],
  }
}

export function traceSqlPanelFor({
  id,
  title,
  datasource,
  query,
  y,
}: Omit<TracePanelInput, "type">): DataPanel {
  return {
    id,
    type: "table",
    title,
    datasource,
    gridPos: { x: 0, y, w: 24, h: 10 },
    queries: [{ query, format: "table" }],
  }
}

export function tracePanelQuery(type: string, search: TraceSearch): string {
  return hasQueryText(type)
    ? textSearchFor(type, search)
    : clickhouseLatestTraceSpans(traceForm(search))
}

export function traceByIdQuery(type: string, traceId: string): string {
  return hasQueryText(type) ? traceId : clickhouseTraceSpans(traceId)
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
