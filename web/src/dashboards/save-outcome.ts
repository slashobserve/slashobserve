import type { DashboardEnvelope } from "./queries"

type MergedFrom = { base_version: number; onto_version: number }

export type SaveOutcome =
  | {
      kind: "saved"
      envelope: DashboardEnvelope
      version: number
      mergedFrom: MergedFrom | null
    }
  | { kind: "proposed"; proposalId: string; revision: number; opened: boolean }

export function saveToast(outcome: SaveOutcome): string {
  if (outcome.kind === "proposed") {
    return outcome.opened
      ? "Proposed for an editor to merge"
      : `Added to your open proposal (revision ${outcome.revision})`
  }
  return outcome.mergedFrom === null
    ? `Saved version ${outcome.version}`
    : `Saved version ${outcome.version}, merged with changes made since version ${outcome.mergedFrom.base_version}`
}
