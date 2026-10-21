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
const dataField = /^__data\.fields(?:\.(.+)|\[\s*(?:"([^"]*)"|'([^']*)')\s*\])$/
const queryParamVar = /^([A-Za-z_][A-Za-z0-9_]*):queryparam$/
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

function tokenValue(
  inner: string,
  scope: LinkScope,
  encode: Encode
): string | null {
  const name = inner.trim()
  if (name === "__url_time_range") {
    if (scope.from === undefined || scope.to === undefined) return null
    return `from=${encode(scope.from)}&to=${encode(scope.to)}`
  }
  const field = dataField.exec(name)
  if (field) {
    const written = field[1] ?? field[2] ?? field[3]
    const value = rowValue(scope, written)
    return value === null ? null : encode(value)
  }
  if (name === "__value.raw") {
    if (scope.value === undefined) return null
    const raw = scope.value === null ? "" : String(scope.value)
    return encode(raw)
  }
  if (name === "__value.text") {
    return scope.valueText === undefined ? null : encode(scope.valueText)
  }
  if (name === "__series.name") {
    return scope.seriesName === undefined ? null : encode(scope.seriesName)
  }
  if (name === "__field.name") {
    return scope.fieldName === undefined ? null : encode(scope.fieldName)
  }
  const queryParam = queryParamVar.exec(name)
  if (queryParam) {
    const values = selectionOf(scope, queryParam[1])
    if (!values) return null
    return values
      .map((value) => `var-${queryParam[1]}=${encode(value)}`)
      .join("&")
  }
  if (plainVar.test(name)) {
    const values = selectionOf(scope, name)
    if (!values) return null
    return encode(values.includes(ALL) ? allLabel : values.join(","))
  }
  return null
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
  const href = interpolate(link.url, scope, encodeURIComponent).trim()
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
