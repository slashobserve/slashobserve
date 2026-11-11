import type { Folder } from "./queries"

export function moveChoices(
  folders: readonly Folder[],
  currentFolderId: string | null
): Folder[] {
  return folders.filter(
    (folder) =>
      (folder.permissions.edit && !folder.personal) ||
      folder.id === currentFolderId
  )
}

export function moveLabel(currentIsPersonal: boolean): string {
  return currentIsPersonal ? "Publish to folder" : "Move to folder"
}
