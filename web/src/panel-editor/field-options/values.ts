import { fieldPropDefaults } from "@/spec"

import { parseNumber } from "../display-options/parse"
import {
  formSchema,
  optionPlaceholders,
  valueLabel,
  type FormKey,
} from "./groups"

export function optionDefault(key: string): unknown {
  return (fieldPropDefaults as Record<string, unknown>)[key]
}

export type EntryResult = { ok: true; value: unknown } | { ok: false }

export function parseOptionEntry(key: FormKey, entry: string): EntryResult {
  const trimmed = entry.trim()
  if (trimmed === "") return { ok: true, value: undefined }
  const schema = formSchema(key)
  switch (schema.kind) {
    case "number": {
      const value = parseNumber(trimmed, {
        min: "min" in schema ? schema.min : -Infinity,
        max: "max" in schema ? schema.max : Infinity,
        minExclusive: "minExclusive" in schema,
        integer: schema.integer,
      })
      return value === null ? { ok: false } : { ok: true, value }
    }
    case "text":
    case "color": {
      const tooLong =
        "maxLength" in schema && [...trimmed].length > schema.maxLength
      const unmatched =
        "pattern" in schema && !new RegExp(schema.pattern).test(trimmed)
      return tooLong || unmatched ? { ok: false } : { ok: true, value: trimmed }
    }
    default:
      return { ok: false }
  }
}

export function entryPlaceholder(key: FormKey): string {
  const fallback = optionDefault(key)
  if (fallback !== undefined) return String(fallback)
  return (
    optionPlaceholders[key] ?? (formSchema(key).kind === "number" ? "Auto" : "")
  )
}

export function entryHint(key: FormKey): string {
  const schema = formSchema(key)
  if (schema.kind === "number") {
    const what = schema.integer ? "A whole number" : "A number"
    const min = "min" in schema ? schema.min : undefined
    const max = "max" in schema ? schema.max : undefined
    if (min !== undefined && max !== undefined) {
      return `${what} from ${min} to ${max}.`
    }
    if (min !== undefined) {
      return "minExclusive" in schema
        ? `${what} above ${min}.`
        : `${what} of at least ${min}.`
    }
    return `${what}.`
  }
  if (schema.kind === "color") {
    return "ok, info, warning, critical, text or a #rrggbb color."
  }
  if ("pattern" in schema) {
    return `For example ${optionPlaceholders[key] ?? ""}.`
  }
  if ("maxLength" in schema) {
    return `At most ${schema.maxLength} characters.`
  }
  return ""
}

export function enumDefaultLabel(key: FormKey): string {
  const fallback = optionDefault(key)
  return fallback === undefined
    ? "Default"
    : `Default (${valueLabel(fallback as string | number)})`
}
