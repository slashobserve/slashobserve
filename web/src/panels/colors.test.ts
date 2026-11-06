import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

const tokens = `
  :root { --series-1: #2563eb; --a: #111111; --b: #222222; }
  .dark { --series-1: #60a5fa; }
`

let resolveColor: (color: string) => string
let style: HTMLStyleElement

beforeEach(async () => {
  vi.resetModules()
  ;({ resolveColor } = await import("./colors"))
  style = document.createElement("style")
  style.textContent = tokens
  document.head.append(style)
})

afterEach(() => {
  style.remove()
  document.documentElement.className = ""
  vi.restoreAllMocks()
})

function spyComputedStyle() {
  return vi.spyOn(window, "getComputedStyle")
}

describe("resolveColor", () => {
  it("reads a token once per root class", () => {
    const spy = spyComputedStyle()
    const first = resolveColor("var(--series-1)")
    const second = resolveColor("var(--series-1)")
    expect(first).toBe("#2563eb")
    expect(second).toBe(first)
    expect(spy).toHaveBeenCalledTimes(1)
  })

  it("reads the token again after the theme class changes", () => {
    const spy = spyComputedStyle()
    expect(resolveColor("var(--series-1)")).toBe("#2563eb")
    document.documentElement.classList.add("dark")
    expect(resolveColor("var(--series-1)")).toBe("#60a5fa")
    expect(spy).toHaveBeenCalledTimes(2)
  })

  it("resolves every token in a color string", () => {
    expect(resolveColor("var(--a) var(--b)")).toBe("#111111 #222222")
  })

  it("does not cache a token that reads as empty", () => {
    const spy = spyComputedStyle()
    expect(resolveColor("var(--missing)")).toBe("")
    expect(resolveColor("var(--missing)")).toBe("")
    expect(spy).toHaveBeenCalledTimes(2)
  })
})
