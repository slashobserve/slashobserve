import { isSave } from "./phase"
import type { ToolCall, ToolResult, ToolSpec } from "./session/types"

export type Made = { tool: string; title: string; url: string }

export const maxMadeLines = 10

const noteStart = "\n\n<note>"

function objectOf(text: string): Record<string, unknown> {
  try {
    const value: unknown = JSON.parse(text)
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {}
  } catch {
    return {}
  }
}

function textOf(value: unknown): string {
  return typeof value === "string" ? value.trim() : ""
}

export function madeBy(
  specs: readonly ToolSpec[],
  calls: readonly ToolCall[],
  results: readonly ToolResult[]
): Made[] {
  const made: Made[] = []
  for (const [index, call] of calls.entries()) {
    const spec = specs.find((s) => s.name === call.name)
    const result = results[index]
    if (
      spec?.readOnly !== false ||
      isSave(call.name) ||
      result === undefined ||
      result.isError
    ) {
      continue
    }
    const input = objectOf(call.input)
    const output = objectOf(result.content.split(noteStart)[0] ?? "")
    made.push({
      tool: call.name,
      title: textOf(input.title) || textOf(input.name),
      url: textOf(output.url),
    })
  }
  return made
}

function toolLabel(tool: string): string {
  const words = tool.replaceAll("_", " ")
  return words.charAt(0).toUpperCase() + words.slice(1)
}

function madeLine({ tool, title, url }: Made): string {
  const shown = title.replace(/[[\]]/g, "\\$&")
  let target = ""
  if (shown !== "" && url !== "") {
    target = `: [${shown}](${url})`
  } else if (shown !== "" || url !== "") {
    target = `: ${shown || url}`
  }
  return `- ${toolLabel(tool)}${target}`
}

export function madeAnswer(made: readonly Made[]): string {
  const lines = made.slice(0, maxMadeLines).map(madeLine)
  const more = made.length - lines.length
  if (more > 0) {
    lines.push(`- and ${more} more`)
  }
  return `Done. This run made what you asked for:\n\n${lines.join("\n")}`
}
