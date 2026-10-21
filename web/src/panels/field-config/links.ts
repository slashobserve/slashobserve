import type { Selections } from "@/api/keys"
import { ALL } from "@/dashboards/search"
import type { WireFrame } from "@/dashboards/queries"
import { matchesName } from "@/panels/transforms"
import { linkDefaults, type DataPanel, type Link } from "@/spec"

import type { FieldResolver } from "./resolve"

export type LinkScope = {
  vars: Selections
  from?: string
  to?: string
  row?: { frame: WireFrame; index: number }
  fields?: FieldResolver
  fromMs?: number
  toMs?: number
  labels?: Record<string, string>
  seriesName?: string
  fieldName?: string
  value?: unknown
  valueText?: string
}

export type ResolvedLink = { title: string; href: string; newTab: boolean }

type Encode = (text: string) => string

const dataTokens = ["${__data.", "${__value", "${__series", "${__field"]
const allowedProtocols = new Set(["http:", "https:", "mailto:"])
const token = /\$\{([^}]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)/g
const wholeToken = new RegExp(`^(?:${token.source})$`)
const dataField = /^__data\.fields(?:\.(.+)|\[\s*(?:"([^"]*)"|'([^']*)')\s*\])$/
const formatSuffix = /^(.+):(raw|text|csv|percentencode|queryparam)$/
const labelPrefix = "__field.labels."
const plainVar = /^[A-Za-z_][A-Za-z0-9_]*$/
const allLabel = "All"

const identity: Encode = (text) => text

export function isDataLink(link: Link): boolean {
  return dataTokens.some(
    (marker) => link.url.includes(marker) || link.title.includes(marker)
  )
}

function rowValue(scope: LinkScope, name: string): string | null {
  const { row } = scope
  if (!row) return null
  const field = row.frame.fields.find(
    (candidate) =>
      matchesName(row.frame, candidate, name) ||
      scope.fields?.displayName(row.frame, candidate) === name
  )
  if (!field || row.index < 0 || row.index >= field.values.length) return null
  const value = field.values[row.index]
  return value === null || value === undefined ? "" : String(value)
}

function selectionOf(scope: LinkScope, name: string): string[] | null {
  return Object.hasOwn(scope.vars, name) ? scope.vars[name] : null
}

function valuesOf(name: string, scope: LinkScope): string[] | null {
  const field = dataField.exec(name)
  if (field) {
    const value = rowValue(scope, field[1] ?? field[2] ?? field[3])
    return value === null ? null : [value]
  }
  if (name === "__value.raw") {
    if (scope.value === undefined) return null
    return [scope.value === null ? "" : String(scope.value)]
  }
  if (name === "__value.text") {
    return scope.valueText === undefined ? null : [scope.valueText]
  }
  if (name === "__series.name") {
    return scope.seriesName === undefined ? null : [scope.seriesName]
  }
  if (name === "__field.name") {
    return scope.fieldName === undefined ? null : [scope.fieldName]
  }
  if (name.startsWith(labelPrefix)) {
    const key = name.slice(labelPrefix.length)
    const { labels } = scope
    return labels && Object.hasOwn(labels, key) ? [labels[key]] : null
  }
  if (name === "__from") {
    return scope.fromMs === undefined ? null : [String(scope.fromMs)]
  }
  if (name === "__to") {
    return scope.toMs === undefined ? null : [String(scope.toMs)]
  }
  if (plainVar.test(name)) {
    const values = selectionOf(scope, name)
    if (!values) return null
    return values.includes(ALL) ? [allLabel] : values
  }
  return null
}

function formatValues(
  values: string[],
  format: string | undefined,
  encode: Encode
): string {
  switch (format) {
    case "raw":
    case "csv":
      return values.join(",")
    case "text":
      return values.join(" + ")
    case "percentencode":
      return encodeURIComponent(values.join(","))
    default:
      return encode(values.join(","))
  }
}

function tokenValue(
  inner: string,
  scope: LinkScope,
  encode: Encode
): string | null {
  const written = inner.trim()
  if (written === "__url_time_range") {
    if (scope.from === undefined || scope.to === undefined) return null
    return `from=${encode(scope.from)}&to=${encode(scope.to)}`
  }
  const suffix = formatSuffix.exec(written)
  const name = suffix ? suffix[1] : written
  const format = suffix?.[2]
  if (format === "queryparam") {
    const selected = plainVar.test(name) ? selectionOf(scope, name) : null
    if (!selected) return null
    return selected.map((value) => `var-${name}=${encode(value)}`).join("&")
  }
  const values = valuesOf(name, scope)
  return values && formatValues(values, format, encode)
}

function interpolate(text: string, scope: LinkScope, encode: Encode): string {
  return text.replace(
    token,
    (written, braced: string | undefined, bare: string | undefined) =>
      tokenValue(braced ?? bare ?? "", scope, encode) ?? written
  )
}

function isSafeUrl(url: string): boolean {
  try {
    return allowedProtocols.has(new URL(url, location.origin).protocol)
  } catch {
    return false
  }
}

export function resolveLink(link: Link, scope: LinkScope): ResolvedLink | null {
  const encode = wholeToken.test(link.url.trim())
    ? identity
    : encodeURIComponent
  const href = interpolate(link.url, scope, encode).trim()
  if (!isSafeUrl(href)) return null
  return {
    title: interpolate(link.title, scope, identity),
    href,
    newTab: link.newTab ?? linkDefaults.newTab,
  }
}

export function panelLinks(panel: DataPanel, scope: LinkScope): ResolvedLink[] {
  return (panel.links ?? [])
    .filter((link) => !isDataLink(link))
    .flatMap((link) => resolveLink(link, scope) ?? [])
}
