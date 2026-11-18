import { describe, expect, it } from "vitest"

import { panelsSettled, waitForPanels } from "./settle"

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
  it("is false until there is a panel and none is loading", () => {
    expect(panelsSettled(sheet([]))).toBe(false)
    expect(panelsSettled(sheet(["ready", "loading"]))).toBe(false)
    expect(
      panelsSettled(sheet(["ready", "empty", "error", "unsupported"]))
    ).toBe(true)
  })
})

describe("waitForPanels", () => {
  it("resolves true once the last panel has loaded", async () => {
    const root = sheet(["ready", "loading"])
    setTimeout(() => {
      root.querySelectorAll("article")[1].dataset.state = "ready"
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
