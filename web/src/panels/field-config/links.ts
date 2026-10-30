import type { Selections } from "@/api/keys"
import { resolveTime, type TimeOptions } from "@/lib/time"
import type { WireFrame } from "@/dashboards/queries"
import { matchesName } from "@/panels/transforms"
import {
  linkDefaults,
  type DashboardLink,
  type DataPanel,
  type Link,
} from "@/spec"

import type { FieldResolver } from "./resolve"
import {
  formatValues,
  identity,
  selectedValues,
  variableFormats,
  type Encode,
  type VariableFormat,
} from "./variable-format"

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

export type ResolvedLink = {
  title: string
  href: string
  newTab: boolean
  oneClick?: true
}

const dataTokens = ["${__data.", "${__value", "${__series", "${__field"]
const allowedProtocols = new Set(["http:", "https:", "mailto:"])
const token = /\$\{([^}]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)/g
const wholeToken = new RegExp(`^(?:${token.source})$`)
const dataField = /^__data\.fields(?:\.(.+)|\[\s*(?:"([^"]*)"|'([^']*)')\s*\])$/
const formatSuffix = new RegExp(`^(.+):(${variableFormats.join("|")})$`)
const labelPrefix = "__field.labels."
const plainVar = /^[A-Za-z_][A-Za-z0-9_]*$/

const unresolvedRange = /\$\{?__(?:from|to)\b/

export function oneClickLink(links: ResolvedLink[]): ResolvedLink | undefined {
  return links.length === 1 && links[0].oneClick ? links[0] : undefined
}

export function isDataLink(link: Link): boolean {
  return (
    link.oneClick === true ||
    dataTokens.some(
      (marker) => link.url.includes(marker) || link.title.includes(marker)
    )
  )
}

function rowValue(scope: LinkScope, name: string): string | null {
  const { row } = scope
  if (!row) return null
  const { fields } = row.frame
  const field =
    fields.find((candidate) => candidate.name === name) ??
    fields.find(
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
    return selectedValues(scope.vars, name)
  }
  return null
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
  const format = suffix?.[2] as VariableFormat | undefined
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
    ...(link.oneClick ? { oneClick: true as const } : {}),
  }
}

function resolvedLinks(links: Link[], scope: LinkScope): ResolvedLink[] {
  return links
    .flatMap((link) => resolveLink(link, scope) ?? [])
    .filter((link) => !unresolvedRange.test(link.href))
}

export function panelLinks(panel: DataPanel, scope: LinkScope): ResolvedLink[] {
  return resolvedLinks(
    (panel.links ?? []).filter((link) => !isDataLink(link)),
    scope
  )
}

export function appendLinkParams(
  href: string,
  options: Pick<DashboardLink, "keepTime" | "includeVars">,
  scope: Pick<LinkScope, "vars" | "from" | "to">
): string {
  if (/^mailto:/i.test(href)) return href
  const params: [string, string][] = []
  if (options.keepTime && scope.from !== undefined && scope.to !== undefined) {
    params.push(["from", scope.from], ["to", scope.to])
  }
  if (options.includeVars) {
    for (const [name, values] of Object.entries(scope.vars)) {
      for (const value of values) params.push([`var-${name}`, value])
    }
  }
  if (params.length === 0) return href
  const hashAt = href.indexOf("#")
  const path = hashAt < 0 ? href : href.slice(0, hashAt)
  const hash = hashAt < 0 ? "" : href.slice(hashAt)
  const query = params
    .map(([key, value]) => `${key}=${encodeURIComponent(value)}`)
    .join("&")
  const joiner = !path.includes("?") ? "?" : /[?&]$/.test(path) ? "" : "&"
  return `${path}${joiner}${query}${hash}`
}

export function dashboardLinks(
  links: DashboardLink[] | undefined,
  scope: LinkScope,
  now: number,
  timeOptions: TimeOptions
): ResolvedLink[] {
  const fromMs =
    scope.from === undefined
      ? NaN
      : resolveTime(scope.from, now, false, timeOptions)
  const toMs =
    scope.to === undefined ? NaN : resolveTime(scope.to, now, true, timeOptions)
  const full = {
    ...scope,
    fromMs: Number.isNaN(fromMs) ? undefined : fromMs,
    toMs: Number.isNaN(toMs) ? undefined : toMs,
  }
  return (links ?? []).flatMap((link) => {
    if (link.url === undefined) return []
    const resolved = resolveLink(
      { title: link.title, url: link.url, newTab: link.newTab },
      full
    )
    if (!resolved || unresolvedRange.test(resolved.href)) return []
    return [{ ...resolved, href: appendLinkParams(resolved.href, link, scope) }]
  })
}
