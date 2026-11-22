import type { Folder } from "@/folders/queries"

export function auditFolderOf(
  folders: readonly Folder[] | undefined
): Folder | null {
  return folders?.find((folder) => folder.audit) ?? null
}
