import { afterEach, describe, expect, it, vi } from "vitest"

import { must } from "@/panels/must"

import { stubRects } from "./anchor-harness"
import { valueAnchorLayout } from "./value-layout"

function container(keys: string[]): HTMLElement {
  const element = document.createElement("div")
  element.append(document.createElement("span"))
  for (const key of keys) {
    const child = document.createElement("div")
    child.dataset.valueKey = key
    const inner = document.createElement("p")
    child.append(inner, document.createElement("p"))
    element.append(child)
  }
  element.append(document.createElement("span"))
  return element
}

const values = [
  { key: "a", name: "cpu", labels: { instance: "a" }, text: "62%" },
  { key: "b", name: "memory", labels: undefined, text: "41%" },
  { key: "c", name: "disk", labels: undefined, text: "7%" },
]

const body = { width: 200, height: 100 }

afterEach(() => vi.restoreAllMocks())

describe("valueAnchorLayout", () => {
  it("pairs each drawn value with its own element, whatever else sits among the children", () => {
    const element = container(["a", "b", "c"])
    stubRects((el) =>
      el === element
        ? { x: 0, y: 0, width: 200, height: 100 }
        : el.getAttribute("data-value-key") === "b"
          ? { x: 10, y: 40, width: 50, height: 20 }
          : undefined
    )
    const layout = valueAnchorLayout(element, values, body)
    expect(layout.body).toEqual(body)
    expect(layout.targets.map(({ name, text }) => [name, text])).toEqual([
      ["cpu", "62%"],
      ["memory", "41%"],
      ["disk", "7%"],
    ])
    expect(must(layout.targets[1]).rect).toEqual({
      x: 10,
      y: 40,
      width: 50,
      height: 20,
    })
    expect(must(layout.targets[0]).labels).toEqual({ instance: "a" })
  })

  it("leaves out a value that was not drawn", () => {
    const element = container(["a"])
    stubRects((el) =>
      el === element ? { x: 0, y: 0, width: 200, height: 100 } : undefined
    )
    const layout = valueAnchorLayout(element, values, body)
    expect(layout.targets.map(({ name }) => name)).toEqual(["cpu"])
  })

  it("clips a value half scrolled out and drops the rect of one scrolled away", () => {
    const element = container(["a", "b", "c"])
    const rects: Record<
      string,
      { x: number; y: number; width: number; height: number }
    > = {
      a: { x: 0, y: -10, width: 200, height: 40 },
      b: { x: 0, y: 70, width: 200, height: 60 },
      c: { x: 0, y: 140, width: 200, height: 40 },
    }
    stubRects((el) =>
      el === element
        ? { x: 0, y: 0, width: 200, height: 100 }
        : rects[el.getAttribute("data-value-key") ?? ""]
    )
    const layout = valueAnchorLayout(element, values, body)
    expect(layout.targets.map(({ rect }) => rect)).toEqual([
      { x: 0, y: 0, width: 200, height: 30 },
      { x: 0, y: 70, width: 200, height: 30 },
      null,
    ])
  })
})
