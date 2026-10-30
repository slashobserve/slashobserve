import type { Folder } from "@/folders/queries"

export const automaticFolder = ""

export function folderChoices(folders: readonly Folder[]): string[] {
  return folders
    .filter((folder) => !folder.general && folder.permissions.create)
    .map((folder) => folder.title)
}

export function requestedFolder(
  selected: string,
  choices: readonly string[]
): string | undefined {
  return choices.includes(selected) ? selected : undefined
}
