import type { InputMode } from "@/ask/input-mode"

export function modeOnSavedSearch(
  mode: InputMode,
  before: string | undefined,
  now: string | undefined
): InputMode {
  return now !== undefined && now !== before && mode === "ask" ? "code" : mode
}
