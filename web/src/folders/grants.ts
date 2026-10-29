import type { components } from "@/api/schema.gen"

export type Level = components["schemas"]["FolderLevel"]
export type Entry = components["schemas"]["FolderPermissionEntry"]
type Entries = components["schemas"]["FolderPermissions"]["entries"]

export type PutEntry = {
  user_id?: string
  email?: string
  group?: string
  level: Level
}

export type GranteeInput = { email: string } | { group: string }

export type GrantChange =
  | { kind: "add"; grantee: GranteeInput; level: Level }
  | { kind: "setLevel"; entry: Entry; level: Level }
  | { kind: "remove"; entry: Entry }
  | { kind: "clear" }

const levels: readonly Level[] = ["view", "edit", "admin"]

export const levelLabels: Record<Level, string> = {
  view: "View",
  edit: "Edit",
  admin: "Admin",
}

const maxGroupBytes = 256
const lowRoleAdminReason = "Needs the Editor role or higher"

function hasControlCharacter(text: string): boolean {
  return Array.from(text).some((char) => {
    const code = char.codePointAt(0) ?? 0
    return code < 0x20 || code === 0x7f
  })
}

export function parseGrantee(text: string): GranteeInput | { error: string } {
  const trimmed = text.trim()
  if (trimmed === "") return { error: "Enter an email or a group name." }
  if (trimmed.includes("@")) {
    if (trimmed.startsWith("@") || trimmed.endsWith("@")) {
      return { error: "Enter a full email address, like name@example.com." }
    }
    return { email: trimmed.toLowerCase() }
  }
  if (new TextEncoder().encode(trimmed).length > maxGroupBytes) {
    return { error: "A group name can be at most 256 bytes." }
  }
  if (hasControlCharacter(trimmed)) {
    return { error: "A group name cannot contain control characters." }
  }
  return { group: trimmed }
}

export function entryKey(entry: Entry): string {
  return entry.kind === "user" && entry.user !== undefined
    ? `user:${entry.user.id}`
    : `group:${entry.group ?? ""}`
}

function putEntry(entry: Entry, level: Level = entry.level): PutEntry {
  return entry.kind === "user" && entry.user !== undefined
    ? { user_id: entry.user.id, level }
    : { group: entry.group ?? "", level }
}

function withGrantee(
  entries: Entries,
  grantee: GranteeInput,
  level: Level
): PutEntry[] {
  const existing = entries.find((entry) =>
    "email" in grantee
      ? entry.user?.email.toLowerCase() === grantee.email
      : entry.kind === "group" && entry.group === grantee.group
  )
  if (existing !== undefined) {
    return entries.map((entry) =>
      entry === existing ? putEntry(entry, level) : putEntry(entry)
    )
  }
  return [...entries.map((entry) => putEntry(entry)), { ...grantee, level }]
}

export function entriesForPut(
  current: Entries,
  change: GrantChange
): PutEntry[] {
  switch (change.kind) {
    case "add":
      return withGrantee(current, change.grantee, change.level)
    case "setLevel": {
      const key = entryKey(change.entry)
      return current.map((entry) =>
        putEntry(entry, entryKey(entry) === key ? change.level : entry.level)
      )
    }
    case "remove": {
      const key = entryKey(change.entry)
      return current
        .filter((entry) => entryKey(entry) !== key)
        .map((entry) => putEntry(entry))
    }
    case "clear":
      return []
  }
}

export function levelOptions(
  target: Entry | GranteeInput
): { level: Level; disabledReason: string | null }[] {
  const role = "kind" in target ? target.user?.role : undefined
  const capped = role === "viewer" || role === "commenter"
  return levels.map((level) => ({
    level,
    disabledReason: capped && level === "admin" ? lowRoleAdminReason : null,
  }))
}
