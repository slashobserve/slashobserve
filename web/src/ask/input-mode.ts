import { useState } from "react"

export type InputMode = "ask" | "query"

const inputModeKey = "slashobserve-input-mode"

export function parseStoredMode(raw: string | null): InputMode | null {
  return raw === "ask" || raw === "query" ? raw : null
}

export function initialMode({
  agentAvailable,
  stored,
  hasQuery,
  hasAsk,
}: {
  agentAvailable: boolean
  stored: InputMode | null
  hasQuery: boolean
  hasAsk: boolean
}): InputMode {
  if (!agentAvailable || hasQuery) return "query"
  if (hasAsk) return "ask"
  return stored ?? "ask"
}

export function nextMode(mode: InputMode, agentAvailable: boolean): InputMode {
  if (!agentAvailable) return "query"
  return mode === "ask" ? "query" : "ask"
}

function readStoredMode(): InputMode | null {
  try {
    return parseStoredMode(localStorage.getItem(inputModeKey))
  } catch {
    return null
  }
}

function writeStoredMode(mode: InputMode) {
  try {
    localStorage.setItem(inputModeKey, mode)
  } catch {
    return
  }
}

export function useInputMode({
  agentAvailable,
  hasQuery,
  hasAsk,
}: {
  agentAvailable: boolean
  hasQuery: boolean
  hasAsk: boolean
}): [InputMode, (next: InputMode, remember?: boolean) => void] {
  const [opening] = useState(() => ({
    stored: readStoredMode(),
    hasQuery,
    hasAsk,
  }))
  const [chosen, setChosen] = useState<InputMode | null>(null)
  const mode = agentAvailable
    ? (chosen ?? initialMode({ agentAvailable, ...opening }))
    : "query"

  function setMode(next: InputMode, remember = true) {
    setChosen(next)
    if (remember) writeStoredMode(next)
  }

  return [mode, setMode]
}
