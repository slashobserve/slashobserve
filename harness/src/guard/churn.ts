import {
  maxChangesPerPanel,
  maxChurnedSavesInRow,
  messageReplacement,
} from "./text"

export type Edits = {
  panels: string[]
  added: string[]
  deleted: string[]
  settings: boolean
}

export type Churn = { changes: Record<string, number>; inRow: number }

const settingsSubject = ""

export function noteChurn(
  churn: Churn,
  edits: Edits | null | undefined
): { note: string; stop: boolean } {
  const subjects = [...(edits?.panels ?? [])]
  if (edits?.settings === true) {
    subjects.push(settingsSubject)
  }
  const added = edits?.added ?? []
  const replaced = (edits?.deleted ?? []).length > 0
  let progressed = false
  for (const id of subjects) {
    const changes = (churn.changes[id] ?? 0) + 1
    churn.changes[id] = changes
    progressed ||=
      changes <= maxChangesPerPanel && !(replaced && added.includes(id))
  }
  if (progressed) {
    churn.inRow = 0
    return { note: "", stop: false }
  }
  churn.inRow++
  const stop = churn.inRow > maxChurnedSavesInRow
  if (replaced && added.length > 0) {
    return { note: messageReplacement, stop }
  }
  let subject = subjects[0] ?? "the build"
  if (subject === settingsSubject) {
    subject = "the dashboard settings"
  }
  return {
    note: `This save changed no panel that still needs work. Leave ${subject} as it is, save the next panel, or set finish: true when every planned panel is built.`,
    stop,
  }
}
