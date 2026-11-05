import { describe, expect, it } from "vitest"

import type { Folder } from "@/folders/queries"

import { publishTargets } from "./publish"

function folder(id: string, extra: Partial<Folder> = {}): Folder {
  return {
    id,
    title: id,
    general: false,
    personal: false,
    restricted: false,
    dashboard_count: 0,
    grantees: [],
    grantee_count: 0,
    created_by: null,
    permissions: { comment: true, edit: true, manage: false, create: true },
    level: "edit",
    ...extra,
  }
}

const viewOnly: Folder["permissions"] = {
  comment: false,
  edit: false,
  manage: false,
  create: false,
}

function ids(folders: Folder[]): string[] {
  return publishTargets(folders).map((target) => target.id)
}

describe("publishTargets", () => {
  it("leaves out a personal folder", () => {
    expect(ids([folder("mine", { personal: true }), folder("ops")])).toEqual([
      "ops",
    ])
  })

  it("leaves out a folder the caller can only view", () => {
    expect(
      ids([folder("ro", { permissions: viewOnly }), folder("ops")])
    ).toEqual(["ops"])
  })

  it("includes General when the caller can edit it", () => {
    expect(
      ids([
        folder("general", { general: true }),
        folder("locked", { general: true, permissions: viewOnly }),
      ])
    ).toEqual(["general"])
  })

  it("keeps the order it was given", () => {
    expect(ids([folder("b"), folder("a"), folder("c")])).toEqual([
      "b",
      "a",
      "c",
    ])
  })

  it("is empty when nothing qualifies", () => {
    expect(ids([folder("mine", { personal: true })])).toEqual([])
  })
})
