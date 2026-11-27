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
  dashboardTitle: string | null
  proposalTitle: string | null
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

export function conflictText(conflict: MergeConflict, label: string): string {
  if (conflict.reason === "invalid") return conflict.message
  const unit =
    conflict.kind === "panel"
      ? `Panel "${label}"`
      : conflict.kind === "variable"
        ? `Variable "${conflict.id}"`
        : `The dashboard's ${conflict.id} setting`
  return conflict.reason === "both_changed"
    ? `${unit} was changed on the dashboard and in this proposal.`
    : `${unit} was removed on one side and changed on the other.`
}

export function changeRows(
  proposal: Pick<Proposal, "spec" | "changes" | "conflicts">,
  latestTitles: Map<string, string>
): ChangeRow[] {
  const proposalTitles = panelTitles(proposal.spec)
  const titles = [proposalTitles, latestTitles]
  const sideTitles = (kind: ChangeKind, id: string) =>
    kind === "panel"
      ? {
          dashboardTitle: latestTitles.get(id) ?? null,
          proposalTitle: proposalTitles.get(id) ?? null,
        }
      : { dashboardTitle: null, proposalTitle: null }
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
      ...sideTitles(kind, id),
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
      ...sideTitles(conflict.kind, conflict.id),
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

export function mergeHint(
  proposal: Pick<Proposal, "mergeability">,
  missing: readonly string[]
): string | null {
  if (proposal.mergeability === "blocked") {
    return "Some changes cannot be merged; discard this proposal or ask its author to fix it"
  }
  if (missing.length > 0) return "Choose a side for each conflict first"
  return null
}

export function droppedText(
  proposal: Pick<Proposal, "spec" | "conflicts">,
  picks: Picks
): string | null {
  const titles = [panelTitles(proposal.spec)]
  const units = proposal.conflicts
    .filter(
      (conflict) =>
        conflict.reason !== "invalid" && picks[unitKey(conflict)] === "ours"
    )
    .map((conflict) =>
      conflict.kind === "panel"
        ? `panel "${labelOf(conflict.kind, conflict.id, titles)}"`
        : conflict.kind === "variable"
          ? `variable "${conflict.id}"`
          : `the ${conflict.id} setting`
    )
  if (units.length === 0) return null
  return `The proposed change to ${units.join(", ")} was dropped; the dashboard keeps its own.`
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
        return "Merged without a new version; nothing from this proposal was added to the dashboard"
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

export function dashboardLinkOf(proposal: ProposalSummary): {
  version: number | null
  label: string
} {
  if (proposal.state === "merged") {
    if (proposal.merged_version === null) {
      return { version: null, label: "Open the dashboard" }
    }
    return {
      version: proposal.merged_version,
      label: `Open the dashboard at version ${proposal.merged_version}`,
    }
  }
  return {
    version: proposal.base_version,
    label: `Open the dashboard at version ${proposal.base_version}`,
  }
}
