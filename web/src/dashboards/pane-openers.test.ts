import { afterEach, describe, expect, it } from "vitest"

import {
  activityButtonId,
  activityCardId,
  controlsButtonId,
  openPaneHeading,
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

describe("openPaneHeading", () => {
  it("returns the heading that labels the open pane", () => {
    const pane = document.createElement("aside")
    pane.id = activityCardId
    pane.setAttribute("aria-labelledby", "heading")
    const heading = document.createElement("h2")
    heading.id = "heading"
    pane.append(heading)
    document.body.append(pane)
    expect(openPaneHeading()).toBe(heading)
  })

  it("returns null when no pane is open", () => {
    expect(openPaneHeading()).toBeNull()
  })
})
