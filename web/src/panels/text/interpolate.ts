import type { Selections } from "@/api/keys"
import { ALL } from "@/dashboards/search"

const variableToken =
  /\$\{([A-Za-z_][A-Za-z0-9_]*)(?::raw)?\}|\$([A-Za-z_][A-Za-z0-9_]*)/g
const asciiPunctuation = /[!-/:-@[-`{-~]/g
const whitespaceRun = /\s+/g
const autolinkStarts = /:(?=\/\/)|@|www(?=\.)/gi
const wordJoiner = "\u2060"
const allLabel = "All"

function escapeMarkdown(value: string): string {
  return value
    .replace(whitespaceRun, " ")
    .replace(autolinkStarts, (start) => start + wordJoiner)
    .replace(asciiPunctuation, (char) => `\\${char}`)
}

function valueText(values: string[]): string {
  if (values.includes(ALL)) return allLabel
  return values.map(escapeMarkdown).join(", ")
}

export function interpolateText(content: string, vars: Selections): string {
  return content.replace(
    variableToken,
    (written, braced: string | undefined, bare: string | undefined) => {
      const name = braced ?? bare ?? ""
      return Object.hasOwn(vars, name) ? valueText(vars[name]) : written
    }
  )
}
