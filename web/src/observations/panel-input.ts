import { resolveTime } from "@/lib/time"
import type { DataPanel } from "@/spec"

const variableReference = /\$\{?[A-Za-z_]\w*/

function holdsVariable(value: unknown): boolean {
  if (typeof value === "string") return variableReference.test(value)
  if (Array.isArray(value)) return value.some(holdsVariable)
  if (typeof value === "object" && value !== null) {
    return Object.values(value).some(holdsVariable)
  }
  return false
}

export function panelUsesVariables(panel: DataPanel): boolean {
  return holdsVariable(panel.queries) || holdsVariable(panel.datasource)
}

export function blockRange(
  from: string | undefined,
  to: string | undefined,
  now: number
): { from: string | null; to: string | null } {
  const none = { from: null, to: null }
  if (from === undefined || to === undefined) return none
  const start = resolveTime(from, now, false)
  const end = resolveTime(to, now, true)
  if (Number.isNaN(start) || Number.isNaN(end) || start >= end) return none
  return {
    from: new Date(start).toISOString(),
    to: new Date(end).toISOString(),
  }
}

export function blockPanelFrom(panel: DataPanel): DataPanel {
  const copy = { ...panel }
  delete copy.repeat
  return copy
}
