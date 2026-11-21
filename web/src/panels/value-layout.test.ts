import { describe, expect, it } from "vitest"

import { valueAnchorLayout } from "./value-layout"

function container(count: number): HTMLElement {
  const element = document.createElement("div")
  for (let index = 0; index < count; index++) {
    element.append(document.createElement("div"))
  }
  return element
}

const values = [
  { name: "cpu", labels: { instance: "a" }, text: "62%" },
  { name: "memory", labels: undefined, text: "41%" },
]

describe("valueAnchorLayout", () => {
  it("pairs each drawn child with the value it shows", () => {
    const layout = valueAnchorLayout(container(2), values, {
      width: 200,
      height: 100,
    })
    expect(layout.body).toEqual({ width: 200, height: 100 })
    expect(layout.targets.map(({ name, text }) => [name, text])).toEqual([
      ["cpu", "62%"],
      ["memory", "41%"],
    ])
    expect(layout.targets[0].labels).toEqual({ instance: "a" })
  })

  it("leaves out a value that was not drawn", () => {
    const layout = valueAnchorLayout(container(1), values, {
      width: 200,
      height: 100,
    })
    expect(layout.targets.map(({ name }) => name)).toEqual(["cpu"])
  })
})
