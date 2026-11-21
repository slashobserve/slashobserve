import { describe, expect, test } from "vitest"

import type { Folder } from "@/folders/queries"

import { auditFolderOf } from "./audit-folder"

function folder(id: string, title: string, audit: boolean): Folder {
  return {
    id,
    title,
    general: false,
    personal: false,
    restricted: true,
    admins_only: true,
    audit,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: true, create: true },
    level: "admin",
  }
}

describe("auditFolderOf", () => {
  test("finds the folder the server flags, whatever it is called now", () => {
    const flagged = folder("f2", "Records", true)
    expect(auditFolderOf([folder("f1", "Audit", false), flagged])).toBe(flagged)
  })

  test("is null when no folder is flagged or the list has not loaded", () => {
    expect(auditFolderOf([folder("f1", "Audit", false)])).toBeNull()
    expect(auditFolderOf([])).toBeNull()
    expect(auditFolderOf(undefined)).toBeNull()
  })
})
