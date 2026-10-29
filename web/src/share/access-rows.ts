import { levelLabels, type Entry } from "@/folders/grants"
import type { components } from "@/api/schema.gen"
import type { Folder, FolderPermissions } from "@/folders/queries"

type Author = components["schemas"]["Author"]

export type AccessRow = {
  key: string
  name: string
  detail: string
  level: string
  person: boolean
}

const instanceAdmins: AccessRow = {
  key: "instance-admins",
  name: "Instance admins",
  detail: "Always have access",
  level: levelLabels.admin,
  person: false,
}

const everyone: AccessRow = {
  key: "everyone",
  name: "Everyone, by role",
  detail: "Viewers view, Commenters comment, Editors edit",
  level: "",
  person: false,
}

function peopleText(count: number): string {
  return `${count} ${count === 1 ? "person" : "people"}`
}

function entryRow(
  entry: Entry,
  meId: string,
  providerName: string | null
): AccessRow {
  if (entry.kind === "user" && entry.user !== undefined) {
    const { id, name, email } = entry.user
    return {
      key: `user:${id}`,
      name,
      detail: id === meId ? `${email}, you` : email,
      level: levelLabels[entry.level],
      person: true,
    }
  }
  const group = entry.group ?? ""
  const source = providerName ? `Group from ${providerName}` : "Group"
  return {
    key: `group:${group}`,
    name: group,
    detail: `${source}, ${peopleText(entry.member_count ?? 0)}`,
    level: levelLabels[entry.level],
    person: false,
  }
}

function creatorRow(creator: Author, meId: string): AccessRow {
  return {
    key: `creator:${creator.kind}:${creator.id}`,
    name: creator.name,
    detail:
      creator.id === meId ? "Created this folder, you" : "Created this folder",
    level: levelLabels.admin,
    person: creator.kind === "user",
  }
}

function isListed(entries: Entry[], creator: Author): boolean {
  return (
    creator.kind === "user" &&
    entries.some((entry) => entry.user?.id === creator.id)
  )
}

export function accessRows(
  folder: Folder,
  permissions: FolderPermissions,
  me: { id: string },
  providerName: string | null
): AccessRow[] {
  if (!permissions.restricted) return [everyone, instanceAdmins]
  const rows = permissions.entries.map((entry) =>
    entryRow(entry, me.id, providerName)
  )
  const creator = folder.created_by
  if (creator !== null && !isListed(permissions.entries, creator)) {
    rows.push(creatorRow(creator, me.id))
  }
  rows.push(instanceAdmins)
  return rows
}
