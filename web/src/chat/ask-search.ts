import { maxPromptLength } from "@/prompts/rules"

export type AskSearch = { prompt?: string }

export function parseAskSearch(search: Record<string, unknown>): AskSearch {
  const value = search.prompt
  if (typeof value !== "string" && typeof value !== "number") return {}
  const prompt = String(value).trim()
  if (prompt === "") return {}
  return { prompt: [...prompt].slice(0, maxPromptLength).join("") }
}
