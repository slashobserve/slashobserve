import type { Selections } from "@/api/keys"
import { ALL } from "@/dashboards/search"

export const variableFormats = [
  "raw",
  "text",
  "csv",
  "pipe",
  "json",
  "percentencode",
  "queryparam",
] as const

export type VariableFormat = (typeof variableFormats)[number]

export type Encode = (text: string) => string

const allLabel = "All"
const strictEscapes = /[!'()*]/g

export const identity: Encode = (text) => text

const encodeStrict: Encode = (text) =>
  encodeURIComponent(text).replace(
    strictEscapes,
    (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`
  )

export function selectedValues(
  vars: Selections,
  name: string
): string[] | null {
  if (!Object.hasOwn(vars, name)) return null
  const values = vars[name]
  return values.includes(ALL) ? [allLabel] : values
}

export function formatValues(
  values: string[],
  format: VariableFormat | undefined,
  encode: Encode
): string {
  switch (format) {
    case "raw":
    case "csv":
      return values.join(",")
    case "text":
      return values.join(" + ")
    case "pipe":
      return values.join("|")
    case "json":
      return JSON.stringify(values)
    case "percentencode":
      return encodeStrict(values.join(","))
    default:
      return encode(values.join(","))
  }
}
