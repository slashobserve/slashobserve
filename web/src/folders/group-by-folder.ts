import type { Folder } from "./queries"

export type FolderGroup<T> = {
  folder: Folder
  rows: T[]
}

export function groupByFolder<T>(
  folders: readonly Folder[],
  items: readonly T[],
  folderIdOf: (item: T) => string
): FolderGroup<T>[] {
  const byFolder = new Map<string, T[]>()
  for (const item of items) {
    const id = folderIdOf(item)
    const group = byFolder.get(id)
    if (group === undefined) byFolder.set(id, [item])
    else group.push(item)
  }
  return folders.flatMap((folder) => {
    const rows = byFolder.get(folder.id)
    return rows === undefined ? [] : [{ folder, rows }]
  })
}

export function countText(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}
