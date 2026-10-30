import type { Folder } from "@/folders/queries"

const copySuffix = " copy"
const maxSourceCodePoints = 195

export type DuplicateTargets = { choices: Folder[]; initial: string | null }

export function duplicateTitle(title: string): string {
  const kept = [...title.trim()].slice(0, maxSourceCodePoints).join("")
  return `${kept.trimEnd()}${copySuffix}`
}

export function duplicateTargets(
  folders: readonly Folder[],
  sourceFolderId: string
): DuplicateTargets {
  const choices = folders.filter((folder) => folder.permissions.create)
  const initial =
    choices.find((folder) => folder.id === sourceFolderId) ??
    choices.find((folder) => folder.general) ??
    choices[0]
  return { choices, initial: initial?.id ?? null }
}

export function duplicateBlockReason(state: {
  pinned: boolean
  editingLayout: boolean
}): string | null {
  if (state.pinned) return "open the latest version first"
  if (state.editingLayout) return "finish the layout first"
  return null
}
