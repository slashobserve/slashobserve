import type { SavedSearch } from "./queries"

export function folderLabel(
  folder: Pick<SavedSearch["folder"], "personal" | "title">
): string {
  return folder.personal ? "My Dashboards" : folder.title
}
