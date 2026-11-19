import { describe, expect, it } from "vitest"

import { moreLabel } from "./more-menu"

describe("moreLabel", () => {
  it("lists every item for an editor in a shared folder", () => {
    expect(
      moreLabel({ mayEdit: true, canDuplicate: true, folderTv: true })
    ).toBe(
      "More: edit details, open on sign-in, export, print, image, duplicate, TV mode, TV mode for this folder, chart hover, delete"
    )
  })

  it("leaves out the folder TV item in a personal folder", () => {
    expect(
      moreLabel({ mayEdit: true, canDuplicate: true, folderTv: false })
    ).not.toContain("this folder")
  })

  it("lists only what a viewer can use", () => {
    expect(
      moreLabel({ mayEdit: false, canDuplicate: false, folderTv: true })
    ).toBe(
      "More: open on sign-in, export, print, image, TV mode, TV mode for this folder, chart hover"
    )
  })
})
