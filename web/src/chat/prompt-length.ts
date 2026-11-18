import { maxPromptLength } from "@/prompts/rules"

const warnFrom = Math.floor(maxPromptLength * 0.9)

export function promptLength(text: string): number {
  return [...text.trim()].length
}

export function promptCounter(
  text: string
): { text: string; over: boolean } | null {
  const length = promptLength(text)
  if (length < warnFrom) return null
  return {
    text: `${length} of ${maxPromptLength}`,
    over: length > maxPromptLength,
  }
}

export const composerPlaceholder =
  "Ask a question or describe a dashboard, like: checkout health by pod, last 6h"

export function emptyPromptText(input: { dashboardId?: string }): string {
  return input.dashboardId === undefined
    ? "Ask a question or describe a dashboard first."
    : "Describe the change you want first."
}

const longPromptLines = 6
const longPromptCharacters = 600

export function isLongPrompt(text: string): boolean {
  return (
    [...text].length > longPromptCharacters ||
    text.split("\n").length > longPromptLines
  )
}
