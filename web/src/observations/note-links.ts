import { validateDashboardSearch } from "@/dashboards/search"
import { parseSearch } from "@/lib/search"
import { logsPanelFor } from "@/logs/add-panel"
import { applyPills, decodePills } from "@/logs/pills"
import {
  validDatasource,
  validateLogsSearch,
  type LogsSearch,
} from "@/logs/search"
import {
  findPanel,
  isDataPanel,
  type Dashboard,
  type DataPanel,
  type Variable,
} from "@/spec"
import { normaliseTraceId, validateTraceViewSearch } from "@/traces/search"

import { newPanelBody } from "./block-rows"
import { blockPanelFrom, blockRange, blockVariables } from "./panel-input"

export type NoteLink =
  | { kind: "logs"; uid: string; query: string; search: LogsSearch }
  | {
      kind: "trace"
      uid: string
      traceId: string
      from?: string | undefined
      to?: string | undefined
    }
  | {
      kind: "panel"
      dashboardId: string
      panelId: string
      from?: string | undefined
      to?: string | undefined
      vars?: Record<string, string[]>
    }

export type PanelBlockBody = ReturnType<typeof newPanelBody>

export type LinkOutcome = { body: PanelBlockBody } | { reason: string }

export function linkLabel(link: NoteLink): string {
  switch (link.kind) {
    case "logs":
      return "Logs search"
    case "trace":
      return `Trace ${link.traceId.slice(0, 8)}`
    case "panel":
      return "Dashboard panel"
  }
}

function clickhouseTraceById(traceId: string): string {
  return `SELECT TraceId AS traceID, SpanId AS spanID, ParentSpanId AS parentSpanID, ServiceName AS serviceName, SpanName AS operationName, Timestamp AS startTime, Duration / 1000000 AS duration, SpanAttributes AS tags FROM otel_traces WHERE TraceId = '${traceId}' ORDER BY startTime LIMIT 1000`
}

export const maxNoteLinks = 10

const addressPattern = /https?:\/\/[^\s<>"'`\])]+/g
const trailingPunctuation = /[.,;:!?]+$/
const historySegment = "versions"

function classify(url: URL): NoteLink | null {
  const [root, id, next] = url.pathname.split("/").filter((part) => part !== "")
  if (id === undefined) return null
  const raw = parseSearch(url.search)
  if (root === "logs") {
    const uid = validDatasource(id)
    const search = validateLogsSearch(raw)
    if (uid === undefined || search.q === undefined) return null
    return { kind: "logs", uid, query: search.q, search }
  }
  if (root === "t") {
    const traceId = normaliseTraceId(id)
    const view = validateTraceViewSearch(raw)
    if (traceId === null || view.ds === undefined) return null
    return {
      kind: "trace",
      uid: view.ds,
      traceId,
      from: view.from,
      to: view.to,
    }
  }
  if (root === "d" && next !== historySegment) {
    const search = validateDashboardSearch(raw)
    const panelId = search.panel ?? search.viewPanel
    if (panelId === undefined) return null
    return {
      kind: "panel",
      dashboardId: id,
      panelId,
      from: search.from,
      to: search.to,
      ...(Object.keys(search.vars).length > 0 ? { vars: search.vars } : {}),
    }
  }
  return null
}

function addressesIn(text: string): URL[] {
  const found: URL[] = []
  for (const match of text.matchAll(addressPattern)) {
    const address = match[0].replace(trailingPunctuation, "")
    if (URL.canParse(address)) found.push(new URL(address))
  }
  return found
}

export function noteLinks(text: string, origin: string): NoteLink[] {
  const links = new Map<string, NoteLink>()
  for (const url of addressesIn(text)) {
    if (url.origin !== origin) continue
    const link = classify(url)
    if (link === null) continue
    const key = JSON.stringify(link)
    if (!links.has(key)) links.set(key, link)
    if (links.size === maxNoteLinks) break
  }
  return [...links.values()]
}

function bodyOf(
  panel: DataPanel,
  from: string | undefined,
  to: string | undefined,
  now: number,
  variables: readonly Variable[] = []
): PanelBlockBody {
  return newPanelBody(
    blockPanelFrom(panel),
    blockRange(from, to, now),
    variables
  )
}

export function logsLinkBody(
  link: Extract<NoteLink, { kind: "logs" }>,
  datasourceType: string | undefined,
  now: number
): PanelBlockBody {
  const pills = decodePills(link.search.f)
  const query =
    datasourceType === undefined
      ? link.query
      : (applyPills(link.query, datasourceType, pills) ?? link.query)
  const panel = logsPanelFor({
    id: "logs",
    title: "Logs",
    datasource: link.uid,
    query,
    y: 0,
  })
  return bodyOf(panel, link.search.from, link.search.to, now)
}

export function traceLinkBody(
  link: Extract<NoteLink, { kind: "trace" }>,
  datasourceType: string | undefined,
  now: number
): PanelBlockBody {
  const panel: DataPanel = {
    id: "trace",
    type: "traces",
    title: "Trace",
    datasource: link.uid,
    gridPos: { x: 0, y: 0, w: 24, h: 20 },
    queries:
      datasourceType === "clickhouse"
        ? [{ query: clickhouseTraceById(link.traceId), format: "table" }]
        : [{ query: link.traceId }],
  }
  return bodyOf(panel, link.from, link.to, now)
}

export function dashboardPanelBody(
  link: Extract<NoteLink, { kind: "panel" }>,
  spec: Dashboard,
  now: number
): LinkOutcome {
  const entry = findPanel(spec, link.panelId)
  if (entry === undefined) {
    return { reason: "The panel is no longer on that dashboard." }
  }
  if (!isDataPanel(entry.panel)) {
    return { reason: "That panel holds no data to copy." }
  }
  const variables = blockVariables(
    entry.panel,
    spec.variables ?? [],
    link.vars ?? {}
  )
  return { body: bodyOf(entry.panel, link.from, link.to, now, variables) }
}
