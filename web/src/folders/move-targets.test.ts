import { describe, expect, it } from "vitest"

import { moveChoices, moveLabel } from "./move-targets"
import type { Folder } from "./queries"

function folder(id: string, extra: Partial<Folder> = {}, edit = true): Folder {
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
    permissions: { comment: true, edit, manage: edit, create: edit },
    level: edit ? "admin" : "comment",
    ...extra,
  }
}

describe("moveChoices", () => {
  it("offers shared folders the caller can edit", () => {
    const folders = [folder("shop"), folder("ops")]
    expect(moveChoices(folders, "shop").map((choice) => choice.id)).toEqual([
      "shop",
      "ops",
    ])
  })

  it("leaves out a folder the caller cannot edit", () => {
    const folders = [folder("shop"), folder("ops", {}, false)]
    expect(moveChoices(folders, null).map((choice) => choice.id)).toEqual([
      "shop",
    ])
  })

  it("leaves out a personal folder unless it is the current one", () => {
    const folders = [folder("mine", { personal: true }), folder("shop")]
    expect(moveChoices(folders, null).map((choice) => choice.id)).toEqual([
      "shop",
    ])
    expect(moveChoices(folders, "mine").map((choice) => choice.id)).toEqual([
      "mine",
      "shop",
    ])
  })

  it("keeps the current folder even when the caller cannot edit it", () => {
    const folders = [folder("ops", {}, false), folder("shop")]
    expect(moveChoices(folders, "ops").map((choice) => choice.id)).toEqual([
      "ops",
      "shop",
    ])
  })
})

describe("moveLabel", () => {
  it("says publish when the current folder is personal", () => {
    expect(moveLabel(true)).toBe("Publish to folder")
  })

  it("says move otherwise", () => {
    expect(moveLabel(false)).toBe("Move to folder")
  })
})
