import type { components } from "@/api/schema.gen"

import type { Folder } from "./queries"

type Author = components["schemas"]["Author"]

export function isOrphanedPersonal(
  folder: Pick<Folder, "personal" | "created_by">,
  principalId: string
): boolean {
  return (
    folder.personal &&
    !(
      folder.created_by?.kind === "user" && folder.created_by.id === principalId
    )
  )
}

export function mayMoveInto(
  folder: Pick<Folder, "personal">,
  creator: Pick<Author, "kind" | "id">,
  principalId: string
): boolean {
  return (
    !folder.personal || (creator.kind === "user" && creator.id === principalId)
  )
}
