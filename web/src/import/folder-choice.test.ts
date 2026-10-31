import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"

import { folderChoices, requestedFolder } from "./folder-choice"

function folder(
  title: string,
  overrides: { general?: boolean; create?: boolean } = {}
): Folder {
  return {
    id: title,
    title,
    general: overrides.general ?? false,
    restricted: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: {
      comment: true,
      edit: overrides.create ?? true,
      manage: false,
      create: overrides.create ?? true,
    },
    level: (overrides.create ?? true) ? "edit" : "comment",
  }
}

describe("folderChoices", () => {
  it("lists the folders the person can create dashboards in, in order", () => {
    const folders = [
      folder("Platform"),
      folder("Payments", { create: false }),
      folder("Checkout"),
    ]
    expect(folderChoices(folders)).toEqual(["Platform", "Checkout"])
  })

  it("leaves out General, which the server treats as the default folder", () => {
    const folders = [folder("General", { general: true }), folder("Platform")]
    expect(folderChoices(folders)).toEqual(["Platform"])
  })

  it("is empty without folders", () => {
    expect(folderChoices([])).toEqual([])
  })
})

describe("requestedFolder", () => {
  const choices = ["Platform", "Checkout"]

  it("sends the chosen folder", () => {
    expect(requestedFolder("Checkout", choices)).toBe("Checkout")
  })

  it("sends nothing for the automatic choice", () => {
    expect(requestedFolder("", choices)).toBeUndefined()
  })

  it("sends nothing when the chosen folder is no longer offered", () => {
    expect(requestedFolder("Deleted", choices)).toBeUndefined()
  })
})
