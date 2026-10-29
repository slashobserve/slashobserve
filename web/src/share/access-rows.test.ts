import { describe, expect, it } from "vitest"

import type { Entry } from "@/folders/grants"
import type { Folder, FolderPermissions } from "@/folders/queries"

import { accessRows, canManageAccess } from "./access-rows"

const priya: Entry = {
  kind: "user",
  user: {
    id: "u-priya",
    name: "Priya",
    email: "priya@example.com",
    role: "editor",
  },
  level: "view",
}
const ishan: Entry = {
  kind: "user",
  user: {
    id: "u-ishan",
    name: "Ishan",
    email: "ishan@example.com",
    role: "admin",
  },
  level: "admin",
}
const sre: Entry = {
  kind: "group",
  group: "sre",
  member_count: 6,
  level: "edit",
}
const solo: Entry = {
  kind: "group",
  group: "oncall",
  member_count: 1,
  level: "view",
}

function folder(createdBy: Folder["created_by"]): Folder {
  return {
    id: "f-1",
    title: "Checkout",
    general: false,
    restricted: true,
    dashboard_count: 3,
    grantees: [],
    grantee_count: 0,
    created_by: createdBy,
    permissions: { comment: true, edit: true, manage: true, create: true },
  }
}

function restricted(...entries: Entry[]): FolderPermissions {
  return { restricted: true, entries }
}

const creatorAuthor = { kind: "user" as const, id: "u-ana", name: "Ana" }
const me = { id: "u-ishan" }

describe("accessRows", () => {
  it("lists every entry, marks the caller, then adds the creator and instance admins", () => {
    const rows = accessRows(
      folder(creatorAuthor),
      restricted(ishan, sre, priya),
      me,
      "Dex"
    )
    expect(
      rows.map(({ name, detail, level }) => [name, detail, level])
    ).toEqual([
      ["Ishan", "ishan@example.com, you", "Admin"],
      ["sre", "Group from Dex, 6 people", "Edit"],
      ["Priya", "priya@example.com", "View"],
      ["Ana", "Created this folder", "Admin"],
      ["Instance admins", "Always have access", "Admin"],
    ])
  })

  it("does not repeat a creator who is already listed", () => {
    const rows = accessRows(
      folder({ kind: "user", id: "u-priya", name: "Priya" }),
      restricted(priya),
      me,
      "Dex"
    )
    expect(rows.map((row) => row.name)).toEqual(["Priya", "Instance admins"])
  })

  it("marks the caller on the creator row", () => {
    const rows = accessRows(
      folder({ kind: "user", id: "u-ishan", name: "Ishan" }),
      restricted(sre),
      me,
      "Dex"
    )
    expect(rows[1]).toMatchObject({
      name: "Ishan",
      detail: "Created this folder, you",
      level: "Admin",
    })
  })

  it("skips the creator when the folder has none", () => {
    const rows = accessRows(folder(null), restricted(priya), me, null)
    expect(rows.map((row) => row.name)).toEqual(["Priya", "Instance admins"])
  })

  it("words a group without a provider name and a one person group", () => {
    const rows = accessRows(folder(null), restricted(sre, solo), me, null)
    expect(rows.map((row) => row.detail)).toEqual([
      "Group, 6 people",
      "Group, 1 person",
      "Always have access",
    ])
  })

  it("shows a folder open to everyone by role", () => {
    const rows = accessRows(
      folder(creatorAuthor),
      { restricted: false, entries: [] },
      me,
      "Dex"
    )
    expect(
      rows.map(({ name, detail, level }) => [name, detail, level])
    ).toEqual([
      [
        "Everyone, by role",
        "Viewers view, Commenters comment, Editors edit",
        "",
      ],
      ["Instance admins", "Always have access", "Admin"],
    ])
  })

  it("gives every row a distinct key", () => {
    const rows = accessRows(
      folder(creatorAuthor),
      restricted(ishan, sre, priya),
      me,
      "Dex"
    )
    expect(new Set(rows.map((row) => row.key)).size).toBe(rows.length)
  })
})

describe("canManageAccess", () => {
  it.each([
    { general: false, canManage: true, expected: true },
    { general: false, canManage: false, expected: false },
    { general: true, canManage: true, expected: false },
    { general: true, canManage: false, expected: false },
  ])(
    "general $general, can manage $canManage shows the link: $expected",
    ({ general, canManage, expected }) => {
      expect(canManageAccess({ general }, canManage)).toBe(expected)
    }
  )
})
