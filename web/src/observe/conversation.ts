export function orderTurns(
  rootId: string,
  revisions: readonly { id: string; created_at: string }[] | undefined
): string[] {
  if (revisions === undefined) return [rootId]
  const sorted = [...revisions].sort((a, b) =>
    a.created_at === b.created_at
      ? compareText(a.id, b.id)
      : compareText(a.created_at, b.created_at)
  )
  const ids = [...new Set(sorted.map((revision) => revision.id))]
  return ids.includes(rootId) ? ids : [rootId, ...ids]
}

function compareText(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

export function stepSummary(steps: readonly { phase: string }[]): string {
  if (steps.length === 0) return "No steps recorded"
  const repairs = steps.filter((step) => step.phase === "repair").length
  const ran = `Ran ${steps.length} ${steps.length === 1 ? "step" : "steps"}`
  return repairs === 0
    ? ran
    : `${ran}, repaired ${repairs} ${repairs === 1 ? "query" : "queries"}`
}

export function shortRequestId(id: string): string {
  return id.slice(0, 8)
}

export function joinNames(names: readonly string[]): string {
  if (names.length <= 1) return names[0] ?? ""
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`
}
