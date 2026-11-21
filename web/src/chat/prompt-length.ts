import { maxPromptLength } from "@/prompts/rules"

const counterRoom = 500

export function promptLength(text: string): number {
  return [...text.trim()].length
}

export type PromptCounter = { text: string; over: boolean }

function characters(count: number): string {
  return count === 1 ? "1 character" : `${count} characters`
}

export function promptCounter(text: string): PromptCounter | null {
  const left = maxPromptLength - promptLength(text)
  if (left > counterRoom) return null
  return left < 0
    ? { text: `${characters(-left)} over`, over: true }
    : { text: `${characters(left)} left`, over: false }
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
