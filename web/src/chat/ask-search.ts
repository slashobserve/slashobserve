import { maxPromptLength } from "@/prompts/rules"

export type AskSearch = { prompt?: string; dashboard?: string }

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function parseAskSearch(search: Record<string, unknown>): AskSearch {
  const value = search.prompt
  if (typeof value !== "string" && typeof value !== "number") return {}
  const prompt = String(value).trim()
  if (prompt === "") return {}
  const dashboard = search.dashboard
  return {
    prompt: [...prompt].slice(0, maxPromptLength).join(""),
    ...(typeof dashboard === "string" && uuid.test(dashboard)
      ? { dashboard }
      : {}),
  }
}
