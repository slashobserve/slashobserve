import type { Folder } from "@/folders/queries"

export function publishTargets(folders: readonly Folder[]): Folder[] {
  return folders.filter((folder) => !folder.personal && folder.permissions.edit)
}

export function publishAudience(
  folder: Pick<Folder, "title" | "restricted" | "grantees" | "grantee_count">
): string {
  if (!folder.restricted) {
    return `Everyone on this server can open it, each with the level their role gives: Viewers view, Commenters comment and Editors edit.`
  }
  const more = folder.grantee_count - folder.grantees.length
  const names = folder.grantees.join(", ")
  const listed =
    more > 0 ? `${names} and ${more} more` : names === "" ? "nobody" : names
  return `${folder.title} is restricted, so only the people and groups on its list (${listed}) and instance admins can open it.`
}
