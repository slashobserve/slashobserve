import { panelTitles } from "@/history/diff-groups"

import type {
  MergeChoice,
  MergeConflict,
  Proposal,
  ProposalSummary,
} from "./queries"

export type ChangeKind = "panel" | "variable" | "setting"

export type ChangeRow = {
  key: string
  kind: ChangeKind
  id: string
  label: string
  change: string
  conflict: MergeConflict | null
}

export type Picks = Record<string, "ours" | "theirs">

const kindOrder: Record<ChangeKind, number> = {
  panel: 0,
  variable: 1,
  setting: 2,
}

export function unitKey(unit: { kind: ChangeKind; id: string }): string {
  return `${unit.kind}:${unit.id}`
}

function labelOf(
  kind: ChangeKind,
  id: string,
  titles: readonly Map<string, string>[]
): string {
  if (kind === "variable") return `Variable ${id}`
  if (kind === "setting") return "Dashboard settings"
  for (const known of titles) {
    const title = known.get(id)
    if (title !== undefined && title !== "") return title
  }
  return id
}

export function changeRows(
  proposal: Pick<Proposal, "spec" | "changes" | "conflicts">,
  latestTitles: Map<string, string>
): ChangeRow[] {
  const titles = [panelTitles(proposal.spec), latestTitles]
  const conflicts = new Map(
    proposal.conflicts.map((conflict) => [unitKey(conflict), conflict])
  )
  const rows = new Map<string, ChangeRow>()
  for (const { kind, id, change } of proposal.changes) {
    const key = unitKey({ kind, id })
    rows.set(key, {
      key,
      kind,
      id,
      label: labelOf(kind, id, titles),
      change,
      conflict: conflicts.get(key) ?? null,
    })
  }
  for (const [key, conflict] of conflicts) {
    if (rows.has(key)) continue
    rows.set(key, {
      key,
      kind: conflict.kind,
      id: conflict.id,
      label: labelOf(conflict.kind, conflict.id, titles),
      change: "conflict",
      conflict,
    })
  }
  return [...rows.values()].sort(
    (a, b) =>
      kindOrder[a.kind] - kindOrder[b.kind] ||
      a.label.localeCompare(b.label) ||
      a.key.localeCompare(b.key)
  )
}

export function choicesFrom(
  conflicts: readonly MergeConflict[],
  picks: Picks
): { choices: MergeChoice[]; missing: string[] } {
  const choices: MergeChoice[] = []
  const missing: string[] = []
  for (const conflict of conflicts) {
    const key = unitKey(conflict)
    const take = picks[key]
    if (conflict.reason === "invalid" || take === undefined) {
      missing.push(key)
      continue
    }
    choices.push({ kind: conflict.kind, id: conflict.id, take })
  }
  return { choices, missing }
}

export function stateLine(proposal: ProposalSummary): string {
  const based = `Open, based on version ${proposal.base_version}`
  switch (proposal.state) {
    case "open":
      return proposal.behind
        ? `${based}. The dashboard is now at version ${proposal.latest_version}.`
        : based
    case "merged":
      if (proposal.merged_version === null) {
        return "Merged; the dashboard already had this change"
      }
      return proposal.closed_by === null
        ? `Merged as version ${proposal.merged_version}`
        : `Merged as version ${proposal.merged_version} by ${proposal.closed_by.name}`
    case "discarded":
      return proposal.closed_by === null
        ? "Discarded"
        : `Discarded by ${proposal.closed_by.name}`
  }
}
