import type { Folder } from "@/folders/queries"

import type { SavedSearch } from "./queries"

export type SavedSearchSection = {
  folder: Folder
  rows: SavedSearch[]
}

export function groupSavedSearches(
  folders: readonly Folder[],
  searches: readonly SavedSearch[]
): SavedSearchSection[] {
  const byFolder = new Map<string, SavedSearch[]>()
  for (const saved of searches) {
    const group = byFolder.get(saved.folder.id)
    if (group === undefined) byFolder.set(saved.folder.id, [saved])
    else group.push(saved)
  }
  return folders.flatMap((folder) => {
    const rows = byFolder.get(folder.id)
    return rows === undefined ? [] : [{ folder, rows }]
  })
}

export function savedSearchCountText(n: number): string {
  return `${n} saved search${n === 1 ? "" : "es"}`
}
