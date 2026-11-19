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

const storageKey = "slashobserve-import-folder"

type ImportedFolder = { title: string; general: boolean; personal: boolean }

export function sharedFolderTitle(folder: ImportedFolder): string | null {
  return folder.general || folder.personal ? null : folder.title
}

export function defaultFolder(
  remembered: string | null,
  choices: readonly string[]
): string {
  return remembered !== null && choices.includes(remembered)
    ? remembered
    : automaticFolder
}

export function readRememberedFolder(): string | null {
  try {
    return localStorage.getItem(storageKey)
  } catch {
    return null
  }
}

export function rememberImportFolder(folder: ImportedFolder) {
  const title = sharedFolderTitle(folder)
  if (title === null) return
  try {
    localStorage.setItem(storageKey, title)
  } catch {
    return
  }
}
