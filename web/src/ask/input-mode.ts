import { useState } from "react"

export type InputMode = "ask" | "builder" | "code"

const inputModeKey = "slashobserve-input-mode"
const cycle: readonly InputMode[] = ["builder", "code", "ask"]

export function parseStoredMode(raw: string | null): InputMode | null {
  if (raw === "query") return "code"
  return raw === "ask" || raw === "builder" || raw === "code" ? raw : null
}

export function initialMode({
  agentAvailable,
  builderCanShow,
  stored,
  hasQuery,
  hasAsk,
}: {
  agentAvailable: boolean
  builderCanShow: boolean
  stored: InputMode | null
  hasQuery: boolean
  hasAsk: boolean
}): InputMode {
  if (hasQuery) return builderCanShow ? "builder" : "code"
  if (hasAsk && agentAvailable) return "ask"
  const fallback = stored ?? (agentAvailable ? "ask" : "code")
  if (fallback === "ask" && !agentAvailable) return "code"
  if (fallback === "builder" && !builderCanShow) return "code"
  return fallback
}

export function nextMode(
  mode: InputMode,
  agentAvailable: boolean,
  builderCanShow = false
): InputMode {
  const available = cycle.filter(
    (candidate) =>
      (candidate !== "ask" || agentAvailable) &&
      (candidate !== "builder" || builderCanShow)
  )
  const index = available.indexOf(mode)
  return available[(index + 1) % available.length]
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
  builderCanShow = false,
  hasQuery,
  hasAsk,
}: {
  agentAvailable: boolean
  builderCanShow?: boolean
  hasQuery: boolean
  hasAsk: boolean
}): [InputMode, (next: InputMode, remember?: boolean) => void] {
  const [opening] = useState(() => ({
    stored: readStoredMode(),
    hasQuery,
    hasAsk,
    builderCanShow,
  }))
  const [chosen, setChosen] = useState<InputMode | null>(null)
  const resolved = chosen ?? initialMode({ agentAvailable, ...opening })
  const mode = resolved === "ask" && !agentAvailable ? "code" : resolved

  function setMode(next: InputMode, remember = true) {
    setChosen(next)
    if (remember) writeStoredMode(next)
  }

  return [mode, setMode]
}
