import { afterEach, describe, expect, it } from "vitest"

import {
  activityButtonId,
  controlsButtonId,
  paneOpener,
  shareButtonId,
} from "./pane-openers"

function mount(id: string): HTMLElement {
  const element = document.createElement("button")
  element.id = id
  document.body.append(element)
  return element
}

afterEach(() => {
  document.body.replaceChildren()
})

describe("paneOpener", () => {
  it("returns the pane's own button when it is on the page", () => {
    const activity = mount(activityButtonId)
    const share = mount(shareButtonId)
    mount(controlsButtonId)
    expect(paneOpener("activity")).toBe(activity)
    expect(paneOpener("share")).toBe(share)
  })

  it("falls back to the Controls button when the pane's button is inside a closed sheet", () => {
    const controls = mount(controlsButtonId)
    expect(paneOpener("activity")).toBe(controls)
    expect(paneOpener("share")).toBe(controls)
  })

  it("returns null when neither is on the page", () => {
    expect(paneOpener("activity")).toBeNull()
  })
})
