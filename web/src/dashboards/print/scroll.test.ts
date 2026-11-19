import { afterEach, describe, expect, it } from "vitest"

import { restoreScroll, saveScroll } from "./scroll"
import { useExportStore } from "./export-store"

function pane(scrollHeight: number, clientHeight: number) {
  const element = document.createElement("main")
  element.dataset.exportPane = ""
  Object.defineProperty(element, "scrollHeight", {
    value: scrollHeight,
    configurable: true,
  })
  Object.defineProperty(element, "clientHeight", {
    value: clientHeight,
    configurable: true,
  })
  document.body.append(element)
  return element
}

function frames(count: number) {
  return new Promise<void>((resolve) => {
    let left = count
    const tick = () => (left-- > 0 ? requestAnimationFrame(tick) : resolve())
    requestAnimationFrame(tick)
  })
}

afterEach(() => {
  document.body.replaceChildren()
  useExportStore.getState().finish()
})

describe("saveScroll and restoreScroll", () => {
  it("puts the scroll pane back where it was", async () => {
    const first = pane(3000, 600)
    first.scrollTop = 900
    const saved = saveScroll()
    first.remove()

    const fresh = pane(3000, 600)
    restoreScroll(saved)
    await frames(2)
    expect(fresh.scrollTop).toBe(900)
  })

  it("waits for the page to be tall enough before it scrolls", async () => {
    const first = pane(3000, 600)
    first.scrollTop = 900
    const saved = saveScroll()
    first.remove()

    const fresh = pane(600, 600)
    restoreScroll(saved)
    await frames(2)
    expect(fresh.scrollTop).toBe(0)
    Object.defineProperty(fresh, "scrollHeight", {
      value: 3000,
      configurable: true,
    })
    await frames(3)
    expect(fresh.scrollTop).toBe(900)
  })
})

describe("the export store", () => {
  it("restores the scroll of the dashboard when the export finishes", async () => {
    const first = pane(3000, 600)
    first.scrollTop = 450
    useExportStore.getState().start("print")
    first.remove()
    const fresh = pane(3000, 600)
    useExportStore.getState().finish()
    await frames(2)
    expect(fresh.scrollTop).toBe(450)
  })
})
