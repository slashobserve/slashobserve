import { describe, expect, it } from "vitest"

import { findPanel, type Dashboard } from "@/spec"

import { withResponsive } from "./responsive"

function dashboard(): Dashboard {
  return {
    specVersion: "0",
    title: "Checkout",
    time: { from: "now-1h", to: "now" },
    panels: [
      {
        id: "top",
        type: "text",
        gridPos: { x: 0, y: 0, w: 12, h: 4 },
        content: "hello",
      },
      {
        id: "row",
        type: "row",
        title: "Row",
        gridPos: { x: 0, y: 4, w: 24, h: 1 },
        panels: [
          {
            id: "child",
            type: "text",
            gridPos: { x: 0, y: 5, w: 12, h: 4 },
            content: "child",
          },
        ],
      },
    ],
  } as Dashboard
}

function responsiveOf(spec: Dashboard, id: string) {
  const found = findPanel(spec, id)
  if (found === undefined || found.panel.type === "row") {
    throw new Error(`no panel ${id}`)
  }
  return found.panel.responsive
}

describe("withResponsive", () => {
  it("sets a top-level panel to false and removes the key when made responsive again", () => {
    const fixed = withResponsive(dashboard(), "top", false)
    expect(responsiveOf(fixed, "top")).toBe(false)

    const again = withResponsive(fixed, "top", true)
    const found = findPanel(again, "top")
    expect(found && "responsive" in found.panel).toBe(false)
    expect(again).toEqual(dashboard())
  })

  it("sets a row child", () => {
    const fixed = withResponsive(dashboard(), "child", false)
    expect(responsiveOf(fixed, "child")).toBe(false)
    expect(responsiveOf(fixed, "top")).toBeUndefined()
  })

  it("returns an equal spec for an unknown id", () => {
    expect(withResponsive(dashboard(), "missing", false)).toEqual(dashboard())
  })

  it("never changes the spec it was given", () => {
    const original = dashboard()
    withResponsive(original, "top", false)
    expect(original).toEqual(dashboard())
  })
})
