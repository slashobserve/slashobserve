import type { Selections } from "@/api/keys"
import type { PreparedFrame } from "@/panels/transforms"

import {
  formatValues,
  identity,
  selectedValues,
  variableFormats,
  type VariableFormat,
} from "./variable-format"

export type DisplayNameScope = {
  frame: PreparedFrame
  fieldName: string
  seriesName: string
  vars: Selections
}

const reference = /\$\{([^}]*)\}|\[\[([^\]]*)\]\]|\$([A-Za-z_][A-Za-z0-9_]*)/g
const formatSuffix = new RegExp(`^(.+):(${variableFormats.join("|")})$`)
const labelPrefix = "__field.labels."
const listJoin = ", "

function allLabels(labels: Record<string, string>): string {
  return Object.keys(labels)
    .map((key) => `${key}="${labels[key]}"`)
    .join(", ")
}

function variableText(written: string, vars: Selections): string | null {
  const suffix = formatSuffix.exec(written)
  const name = suffix ? suffix[1] : written
  const values = selectedValues(vars, name)
  if (!values) return null
  const format = suffix?.[2] as VariableFormat | undefined
  return format ? formatValues(values, format, identity) : values.join(listJoin)
}

function tokenText(inner: string, scope: DisplayNameScope): string | null {
  const name = inner.trim()
  const labels = scope.frame.labels ?? {}
  if (name === "__series.name" || name === "__field.displayName") {
    return scope.seriesName
  }
  if (name === "__field.name") return scope.fieldName
  if (name === "__data.refId") return scope.frame.ref
  if (name === "__field.labels") return allLabels(labels)
  if (name.startsWith(labelPrefix)) {
    const key = name.slice(labelPrefix.length)
    return Object.hasOwn(labels, key) ? labels[key] : null
  }
  return variableText(name, scope.vars)
}

export function interpolateDisplayName(
  template: string,
  scope: DisplayNameScope
): string {
  return template.replace(
    reference,
    (written, braced?: string, bracketed?: string, bare?: string) =>
      tokenText(braced ?? bracketed ?? bare ?? "", scope) ?? written
  )
}
