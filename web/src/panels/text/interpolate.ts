import type { Selections } from "@/api/keys"
import { ALL } from "@/dashboards/search"
import type { TextPanel } from "@/spec"

export type TextMode = NonNullable<TextPanel["mode"]>

const variableToken =
  /\$\{([A-Za-z_][A-Za-z0-9_]*)(?::raw)?\}|\$([A-Za-z_][A-Za-z0-9_]*)/g
const markdownPunctuation = /[!#$%(-/:;=?@[-`{-~]/g
const htmlSpecials = /[&<>"']/g
const whitespaceRun = /\s+/g
const autolinkStarts = /:(?=\/\/)|@|www(?=\.)/gi
const wordJoiner = "⁠"
const allLabel = "All"

const htmlEntities: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
}

function escapeHtml(value: string): string {
  return value.replace(htmlSpecials, (char) => htmlEntities[char] ?? char)
}

function escapeMarkdown(value: string): string {
  return escapeHtml(
    value
      .replace(whitespaceRun, " ")
      .replace(autolinkStarts, (start) => start + wordJoiner)
      .replace(markdownPunctuation, (char) => `\\${char}`)
  )
}

const escapeByMode: Record<TextMode, (value: string) => string> = {
  markdown: escapeMarkdown,
  html: escapeHtml,
  code: (value) => value,
}

export function interpolateText(
  content: string,
  vars: Selections,
  mode: TextMode
): string {
  const escape = escapeByMode[mode]
  return content.replace(
    variableToken,
    (written, braced: string | undefined, bare: string | undefined) => {
      const name = braced ?? bare ?? ""
      const values = Object.hasOwn(vars, name) ? vars[name] : undefined
      if (values === undefined) return written
      if (values.includes(ALL)) return allLabel
      return values.map(escape).join(", ")
    }
  )
}
