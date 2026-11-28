import { describe, expect, it } from "vitest"

import { panelsSettled, slowPanelNames, waitForPanels } from "./settle"

function sheet(states: string[]) {
  const root = document.createElement("div")
  for (const [index, state] of states.entries()) {
    const panel = document.createElement("article")
    panel.dataset.panelId = `p${index}`
    panel.dataset.state = state
    root.append(panel)
  }
  return root
}

describe("panelsSettled", () => {
  it("is false while any panel is loading", () => {
    expect(panelsSettled(sheet(["ready", "loading"]))).toBe(false)
    expect(
      panelsSettled(sheet(["ready", "empty", "error", "unsupported"]))
    ).toBe(true)
  })

  it("is true for a sheet with no panel, so an empty dashboard does not wait", () => {
    expect(panelsSettled(sheet([]))).toBe(true)
  })
})

describe("slowPanelNames", () => {
  it("names the panels that are still loading by their title", () => {
    const root = sheet(["ready", "loading", "loading"])
    const panels = root.querySelectorAll("article")
    panels.forEach((panel, index) => {
      const title = document.createElement("h3")
      title.id = `t${index}`
      title.textContent = `Panel ${index}`
      panel.setAttribute("aria-labelledby", title.id)
      panel.append(title)
    })
    document.body.append(root)
    expect(slowPanelNames(root)).toEqual(["Panel 1", "Panel 2"])
    root.remove()
  })
})

describe("waitForPanels", () => {
  it("resolves true once the last panel has loaded", async () => {
    const root = sheet(["ready", "loading"])
    setTimeout(() => {
      root.querySelectorAll("article")[1]!.dataset.state = "ready"
    }, 150)
    expect(await waitForPanels(root, { timeoutMs: 2000, settleMs: 0 })).toBe(
      true
    )
  })

  it("gives up after the timeout and says so", async () => {
    const root = sheet(["loading"])
    expect(await waitForPanels(root, { timeoutMs: 200, settleMs: 0 })).toBe(
      false
    )
  })
})
