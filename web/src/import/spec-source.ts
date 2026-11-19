export type SpecSource = {
  title: string
  panels: number
  variables: number
  spec: Record<string, unknown>
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function countPanels(panels: unknown[]): number {
  let total = 0
  for (const panel of panels) {
    if (!isRecord(panel)) continue
    if (Array.isArray(panel.panels)) total += countPanels(panel.panels)
    else total += 1
  }
  return total
}

export function readSpecSource(text: string): SpecSource | null {
  if (!text.trimStart().startsWith("{")) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return null
  }
  if (
    !isRecord(parsed) ||
    typeof parsed.specVersion !== "string" ||
    !Array.isArray(parsed.panels)
  ) {
    return null
  }
  return {
    title: typeof parsed.title === "string" ? parsed.title : "",
    panels: countPanels(parsed.panels),
    variables: Array.isArray(parsed.variables) ? parsed.variables.length : 0,
    spec: parsed,
  }
}

export function specSummaryLine(source: SpecSource): string {
  const panels = `${source.panels} panel${source.panels === 1 ? "" : "s"}`
  const variables = `${source.variables} variable${source.variables === 1 ? "" : "s"}`
  return `ObserveSpec dashboard · ${panels} · ${variables}`
}
