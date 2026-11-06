import type { Variable } from "@/spec"

export type PublicMode = "frozen" | "free"

const signedInPanelTypes: ReadonlySet<string> = new Set([
  "dashlist",
  "news",
  "geomap",
])

const viewerChoiceTypes: ReadonlySet<Variable["type"]> = new Set([
  "query",
  "custom",
  "interval",
  "datasource",
])

export function availableOnPublicLink(panelType: string): boolean {
  return !signedInPanelTypes.has(panelType)
}

export function viewerMayChoose(mode: PublicMode, variable: Variable): boolean {
  return mode === "free" && viewerChoiceTypes.has(variable.type)
}

export function viewerVars(
  mode: PublicMode,
  variables: readonly Variable[],
  stored: Record<string, string[]>,
  requested: Record<string, string[]>
): Record<string, string[]> {
  const out = { ...stored }
  for (const variable of variables) {
    const values = requested[variable.name]
    if (values !== undefined && viewerMayChoose(mode, variable)) {
      out[variable.name] = values
    }
  }
  return out
}
