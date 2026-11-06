import type { Folder } from "@/folders/queries"

export function publishTargets(folders: readonly Folder[]): Folder[] {
  return folders.filter((folder) => !folder.personal && folder.permissions.edit)
}
