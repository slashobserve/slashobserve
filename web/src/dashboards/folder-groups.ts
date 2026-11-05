import { levelLabels } from "@/folders/grants"
import { isOrphanedPersonal } from "@/folders/personal"
import type { Folder } from "@/folders/queries"

import type { DashboardSummary } from "./dashboard-columns"
import { listPageSize, pageCountOf, pageRange } from "./list-pages"

export type FolderSection = {
  folder: Folder
  rows: DashboardSummary[]
  empty: boolean
  continued: boolean
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

export function groupPage(
  folders: Folder[],
  rows: DashboardSummary[],
  page: { offset: number; total: number; filtered: boolean }
): FolderSection[] {
  const { offset, total, filtered } = page
  const counted = folders.reduce(
    (sum, folder) => sum + folder.dashboard_count,
    0
  )
  const exact = !filtered && counted === total
  const pageEnd = offset + listPageSize
  const byFolder = rowsByFolder(rows)
  const sections: FolderSection[] = []
  let start = 0
  for (const folder of folders) {
    const onPage = byFolder.get(folder.id) ?? []
    if (onPage.length > 0) {
      sections.push({
        folder,
        rows: onPage,
        empty: false,
        continued: exact && start < offset,
      })
    } else if (
      exact &&
      folder.dashboard_count === 0 &&
      start >= offset &&
      (start < pageEnd || pageEnd >= total)
    ) {
      sections.push({ folder, rows: [], empty: true, continued: false })
    }
    start += folder.dashboard_count
  }
  return sections
}

export function hasPeopleFolders(folders: Folder[]): boolean {
  return folders.some((folder) => !folder.general && !folder.personal)
}

export function accessSummary(folder: Folder, principalId: string): string {
  if (isOrphanedPersonal(folder, principalId)) {
    return "Only Admins: the owner's account was deleted"
  }
  if (folder.personal) return "Only you"
  if (!folder.restricted) return "Everyone, by role"
  const names = folder.grantees.join(", ")
  const more = folder.grantee_count - folder.grantees.length
  return more > 0
    ? `Restricted: ${names}, and ${more} more`
    : `Restricted: ${names}`
}

export function yourAccessText(folder: Folder): string | null {
  return folder.restricted && !folder.personal
    ? `Your access: ${levelLabels[folder.level]}`
    : null
}

function plural(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? "" : "s"}`
}

export function dashboardCountText(n: number): string {
  return plural(n, "dashboard")
}

export function listSummary(
  total: number,
  folderCount: number,
  page: number
): string {
  const pageCount = pageCountOf(total)
  if (pageCount === 1) {
    return `${plural(total, "dashboard")} in ${plural(folderCount, "folder")}`
  }
  const { first, last } = pageRange(page, total)
  return `Page ${page} of ${pageCount}, dashboards ${first}-${last} of ${total}`
}
