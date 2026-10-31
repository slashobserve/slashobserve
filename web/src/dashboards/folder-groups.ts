import { levelLabels } from "@/folders/grants"
import type { Folder } from "@/folders/queries"

import type { DashboardSummary } from "./dashboard-columns"

export type FolderSection = {
  folder: Folder
  rows: DashboardSummary[]
  empty: boolean
}

function rowsByFolder(
  rows: DashboardSummary[]
): Map<string, DashboardSummary[]> {
  const byFolder = new Map<string, DashboardSummary[]>()
  for (const row of rows) {
    const group = byFolder.get(row.folder.id)
    if (group === undefined) byFolder.set(row.folder.id, [row])
    else group.push(row)
  }
  return byFolder
}

export function groupByFolder(
  folders: Folder[],
  rows: DashboardSummary[],
  hasMore: boolean,
  filtered: boolean
): FolderSection[] {
  const byFolder = rowsByFolder(rows)
  const lastLoaded = folders.findLastIndex((folder) => byFolder.has(folder.id))
  const sections: FolderSection[] = []
  folders.forEach((folder, index) => {
    const loaded = byFolder.get(folder.id) ?? []
    if (loaded.length > 0) {
      sections.push({ folder, rows: loaded, empty: false })
      return
    }
    if (filtered || (hasMore && index > lastLoaded)) return
    sections.push({ folder, rows: [], empty: true })
  })
  return sections
}

export function accessSummary(folder: Folder): string {
  if (!folder.restricted) return "Everyone, by role"
  const names = folder.grantees.join(", ")
  const more = folder.grantee_count - folder.grantees.length
  return more > 0
    ? `Restricted: ${names}, and ${more} more`
    : `Restricted: ${names}`
}

export function yourAccessText(folder: Folder): string | null {
  return folder.restricted ? `Your access: ${levelLabels[folder.level]}` : null
}

function plural(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`
}

export function dashboardCountText(n: number): string {
  return plural(n, "dashboard")
}

export function listSummary(total: number, folderCount: number): string {
  return `${plural(total, "dashboard")} in ${plural(folderCount, "folder")}`
}
