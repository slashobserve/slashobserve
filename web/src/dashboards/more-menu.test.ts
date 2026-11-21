import { describe, expect, it } from "vitest"

import { moreLabel } from "./more-menu"

describe("moreLabel", () => {
  it("lists every item for an editor in a shared folder", () => {
    expect(
      moreLabel({
        mayEdit: true,
        mayDelete: true,
        canAddPanel: true,
        canDuplicate: true,
        folderTv: true,
      })
    ).toBe(
      "More: edit details, add panel, edit Spec JSON, open on sign-in, export, dashboards as code, print, image, duplicate, TV mode, TV mode for this folder, chart hover, delete"
    )
  })

  it("leaves out the folder TV item in a personal folder", () => {
    expect(
      moreLabel({
        mayEdit: true,
        mayDelete: true,
        canAddPanel: true,
        canDuplicate: true,
        folderTv: false,
      })
    ).not.toContain("this folder")
  })

  it("leaves out add panel when the dashboard has no room for one", () => {
    expect(
      moreLabel({
        mayEdit: true,
        mayDelete: true,
        canAddPanel: false,
        canDuplicate: true,
        folderTv: true,
      })
    ).not.toContain("add panel")
  })

  it("lists only what a viewer can use", () => {
    expect(
      moreLabel({
        mayEdit: false,
        mayDelete: false,
        canAddPanel: true,
        canDuplicate: false,
        folderTv: true,
      })
    ).toBe(
      "More: open on sign-in, export, dashboards as code, print, image, TV mode, TV mode for this folder, chart hover"
    )
  })

  it("keeps delete but not the content items on a starter dashboard", () => {
    const label = moreLabel({
      mayEdit: false,
      mayDelete: true,
      canAddPanel: true,
      canDuplicate: true,
      folderTv: false,
    })
    expect(label).toContain("duplicate")
    expect(label).toContain("delete")
    expect(label).not.toContain("edit details")
    expect(label).not.toContain("add panel")
  })
})
