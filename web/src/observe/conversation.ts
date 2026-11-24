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

type LatestTurn = {
  id: string
  created_at: string
  status: "queued" | "running" | "succeeded" | "failed"
  failure_code: string | null
  result_version: number | null
}

export function latestTurn(
  root: LatestTurn,
  others: readonly LatestTurn[] | undefined
): LatestTurn {
  return [root, ...(others ?? [])].reduce((latest, turn) =>
    turn.created_at > latest.created_at ||
    (turn.created_at === latest.created_at && turn.id > latest.id)
      ? turn
      : latest
  )
}

function compareText(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

export function costText(cost: number | null | undefined): string | undefined {
  if (cost === null || cost === undefined) return undefined
  if (cost === 0) return "$0.00"
  if (cost < 0.01) return "under $0.01"
  return `about $${cost.toFixed(2)}`
}

export function stepSummary(
  steps: readonly { phase: string }[],
  took?: string,
  cost?: string
): string {
  if (steps.length === 0) return "No steps recorded"
  const retries = steps.filter((step) => step.phase === "repair").length
  const counted = `Ran ${steps.length} ${steps.length === 1 ? "step" : "steps"}`
  const timed = took === undefined ? counted : `${counted} in ${took}`
  const ran = cost === undefined ? timed : `${timed} for ${cost}`
  return retries === 0
    ? ran
    : `${ran}, retried ${retries} ${retries === 1 ? "time" : "times"}`
}

export function shortRequestId(id: string): string {
  return id.slice(0, 8)
}

export function joinNames(names: readonly string[]): string {
  if (names.length <= 1) return names[0] ?? ""
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`
}
