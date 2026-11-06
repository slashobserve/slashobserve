import { useState } from "react"

const promptHistoryKey = "slashobserve-prompt-history"
export const promptHistoryLimit = 20

export type HistoryCursor = { index: number; draft: string }

export const freshCursor: HistoryCursor = { index: -1, draft: "" }

export function parseStoredHistory(raw: string | null): string[] {
  if (raw === null) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  if (!parsed.every((entry) => typeof entry === "string")) return []
  return parsed.slice(0, promptHistoryLimit)
}

export function pushPrompt(
  history: readonly string[],
  prompt: string
): string[] {
  const trimmed = prompt.trim()
  if (trimmed === "") return [...history]
  return [trimmed, ...history.filter((entry) => entry !== trimmed)].slice(
    0,
    promptHistoryLimit
  )
}

export function stepHistory(
  history: readonly string[],
  cursor: HistoryCursor,
  direction: "older" | "newer",
  text: string
): { cursor: HistoryCursor; text: string } | null {
  if (direction === "older") {
    const index = cursor.index + 1
    if (index >= history.length) return null
    const draft = cursor.index === -1 ? text : cursor.draft
    return { cursor: { index, draft }, text: history[index] }
  }
  if (cursor.index === -1) return null
  const index = cursor.index - 1
  return {
    cursor: { index, draft: cursor.draft },
    text: index === -1 ? cursor.draft : history[index],
  }
}

function readHistory(storageKey: string): string[] {
  try {
    return parseStoredHistory(localStorage.getItem(storageKey))
  } catch {
    return []
  }
}

function writeHistory(storageKey: string, entries: readonly string[]) {
  try {
    localStorage.setItem(storageKey, JSON.stringify(entries))
  } catch {
    return
  }
}

export function usePromptHistory(storageKey = promptHistoryKey): {
  entries: string[]
  record: (prompt: string) => void
} {
  const [entries, setEntries] = useState(() => readHistory(storageKey))

  function record(prompt: string) {
    const next = pushPrompt(readHistory(storageKey), prompt)
    writeHistory(storageKey, next)
    setEntries(next)
  }

  return { entries, record }
}
