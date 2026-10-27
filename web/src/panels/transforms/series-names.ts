import type { WireFrame } from "@/dashboards/queries"
import { queryRef, type DataPanel, type PanelQuery } from "@/spec"

import {
  numberFieldCount,
  type PreparedField,
  type PreparedFrame,
} from "./names"

export type DatasourceTypeOf = (uid: string) => string | undefined

type Labels = Record<string, string>
type Kind = "prometheus" | "sql"

type Context = {
  frameNamesDiffer: boolean
  singleLabel: string | null
}

type Source = {
  frame: WireFrame
  query: PanelQuery | undefined
  named: boolean
  kind: Kind
}

type GrafanaField = {
  labels: Labels | undefined
  frame: string | undefined
}

const goEscapes: Record<string, string> = {
  "\\": "\\\\",
  '"': '\\"',
  "\u0007": "\\a",
  "\b": "\\b",
  "\f": "\\f",
  "\n": "\\n",
  "\r": "\\r",
  "\t": "\\t",
  "\v": "\\v",
}

function goQuote(value: string): string {
  let out = ""
  for (const char of value) {
    const code = char.codePointAt(0) ?? 0
    if (goEscapes[char]) out += goEscapes[char]
    else if (code < 0x20 || code === 0x7f) {
      out += `\\x${code.toString(16).padStart(2, "0")}`
    } else out += char
  }
  return `"${out}"`
}

function formatLabels(labels: Labels): string {
  const keys = Object.keys(labels).sort()
  if (keys.length === 0) return ""
  return `{${keys.map((key) => `${key}="${labels[key]}"`).join(", ")}}`
}

function metricNameFromLabels(labels: Labels): string {
  const hasName = Object.hasOwn(labels, "__name__")
  const name = hasName ? labels.__name__ : ""
  const pairs = Object.keys(labels)
    .filter((key) => key !== "__name__")
    .map((key) => `${key}=${goQuote(labels[key])}`)
    .sort()
  if (pairs.length === 0) return hasName ? name : "{}"
  return `${name}{${pairs.join(", ")}}`
}

function renderLegend(template: string, labels: Labels): string {
  return template.replace(/\{\{\s*(.+?)\s*\}\}/g, (_match, key: string) => {
    const label = key.trim()
    return Object.hasOwn(labels, label) ? labels[label] : ""
  })
}

function prometheusLabels(frame: WireFrame): Labels {
  return {
    ...frame.labels,
    ...(frame.name !== undefined ? { __name__: frame.name } : {}),
  }
}

function inferKind(frame: WireFrame): Kind {
  const isPrometheusShape =
    frame.fields.length === 2 &&
    frame.fields[0].type === "time" &&
    frame.fields[1].name === "Value"
  return frame.name !== undefined || isPrometheusShape ? "prometheus" : "sql"
}

function resolveSource(
  panel: DataPanel,
  frame: WireFrame,
  datasourceTypeOf: DatasourceTypeOf | undefined
): Source {
  const query = panel.queries.find(
    (candidate, index) => queryRef(candidate, index) === frame.ref
  )
  const type = datasourceTypeOf?.(query?.datasource ?? panel.datasource)
  // Fallback while datasource types are unknown (list loading or failed).
  const kind =
    type === undefined
      ? inferKind(frame)
      : type === "prometheus"
        ? "prometheus"
        : "sql"
  const named = query?.format !== "table" && query?.format !== "logs"
  return { frame, query, named, kind }
}

function frameLabels({ frame, kind }: Source): Labels | undefined {
  return kind === "prometheus" ? prometheusLabels(frame) : frame.labels
}

function frameName({ frame, kind }: Source): string | undefined {
  return kind === "prometheus" ? undefined : frame.ref
}

function contextOf(sources: Source[]): Context {
  const named = sources.filter((source) => source.named)
  const frameNamesDiffer = named.some(
    (source, index) =>
      index > 0 && frameName(named[index - 1]) !== frameName(source)
  )
  let singleLabel: string | null = null
  let conflicting = false
  for (const source of named) {
    if (source.frame.fields.every((field) => field.type === "time")) continue
    for (const key of Object.keys(frameLabels(source) ?? {})) {
      if (singleLabel === null) singleLabel = key
      else if (singleLabel !== key) conflicting = true
    }
  }
  return { frameNamesDiffer, singleLabel: conflicting ? null : singleLabel }
}

function uniqueName(
  name: string,
  frame: WireFrame,
  fieldIndex: number
): string {
  const field = frame.fields[fieldIndex]
  if (name !== field.name) return name
  const sameName = frame.fields.filter((other) => other.name === name)
  if (sameName.length < 2) return name
  const before = frame.fields
    .slice(0, fieldIndex)
    .filter((other) => other.name === name).length
  return `${name} ${before + 1}`
}

function generic(
  fieldName: string,
  grafana: GrafanaField,
  context: Context,
  frame: WireFrame,
  fieldIndex: number
): string {
  const parts: string[] = []
  let framePushed = false
  if (context.frameNamesDiffer && grafana.frame) {
    parts.push(grafana.frame)
    framePushed = true
  }
  if (fieldName !== "Value") parts.push(fieldName)
  let labelsPushed = false
  const labels = grafana.labels ?? {}
  if (Object.keys(labels).length > 0) {
    const text =
      context.singleLabel === null
        ? formatLabels(labels)
        : (labels[context.singleLabel] ?? "")
    if (text !== "") {
      parts.push(text)
      labelsPushed = true
    }
  }
  if (!framePushed && !labelsPushed && fieldName === "Value" && grafana.frame) {
    parts.push(grafana.frame)
  }
  const name = parts.join(" ") || fieldName || "Value"
  return uniqueName(name, frame, fieldIndex)
}

function prometheusName(
  source: Source,
  context: Context,
  fieldIndex: number
): string {
  const { frame, query } = source
  const labels = prometheusLabels(frame)
  const legend = query?.legend
  let displayName: string
  if (legend === "__auto") {
    displayName =
      Object.keys(labels).length > 0 ? "" : metricNameFromLabels(labels)
  } else if (legend) {
    displayName = renderLegend(legend, labels)
  } else {
    displayName = metricNameFromLabels(labels)
  }
  if (displayName === "{}") displayName = query?.query ?? ""
  if (displayName !== "") return displayName
  const fieldName = legend === "__auto" ? "Value" : (frame.name ?? "Value")
  return generic(
    fieldName,
    { labels, frame: undefined },
    context,
    frame,
    fieldIndex
  )
}

function sqlName(source: Source, context: Context, fieldIndex: number): string {
  const { frame, query } = source
  const field = frame.fields[fieldIndex]
  const legend = query?.legend
  if (legend && legend !== "__auto") {
    const labels = {
      ...frame.labels,
      ...(frame.name ? { __name__: frame.name } : {}),
    }
    const rendered = renderLegend(legend, labels)
    if (rendered !== "") {
      return numberFieldCount(frame) > 1
        ? `${rendered} ${field.name}`
        : rendered
    }
  }
  return generic(
    field.name,
    { labels: frame.labels, frame: frame.ref },
    context,
    frame,
    fieldIndex
  )
}

function stampField(
  source: Source,
  context: Context,
  field: PreparedField,
  fieldIndex: number
): PreparedField {
  if (field.type !== "number") return field
  const seriesName =
    source.kind === "prometheus"
      ? prometheusName(source, context, fieldIndex)
      : sqlName(source, context, fieldIndex)
  return { ...field, seriesName }
}

export function stampSeriesNames(
  panel: DataPanel,
  frames: WireFrame[],
  datasourceTypeOf?: DatasourceTypeOf
): PreparedFrame[] {
  const sources = frames.map((frame) =>
    resolveSource(panel, frame, datasourceTypeOf)
  )
  const context = contextOf(sources)
  return sources.map((source) => ({
    ...source.frame,
    fields: source.named
      ? source.frame.fields.map((field, index) =>
          stampField(source, context, field, index)
        )
      : [...source.frame.fields],
  }))
}
