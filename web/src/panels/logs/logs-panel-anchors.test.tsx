import { act } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { clip } from "../anchors"
import { panelOf, renderPublishing, stubRects } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import { locate } from "./anchors"
import type { LogAnchorLayout } from "./anchors"
import LogsPanel from "./logs-panel"

const frames: PreparedFrame[] = [
  {
    ref: "A",
    fields: [
      { name: "Time", type: "time", values: [1_000, 2_000, 3_000] },
      { name: "Body", type: "string", values: ["first", "second", "third"] },
    ],
  },
]

function lines(count: number): PreparedFrame[] {
  return [
    {
      ref: "A",
      fields: [
        {
          name: "Time",
          type: "time",
          values: Array.from(
            { length: count },
            (_, index) => 1_000 * (index + 1)
          ),
        },
        {
          name: "Body",
          type: "string",
          values: Array.from({ length: count }, (_, index) => `line ${index}`),
        },
      ],
    },
  ]
}

function scroller(container: HTMLElement): HTMLElement {
  const list = container.querySelector("ul[data-logs-count]")
  return list?.parentElement as HTMLElement
}

function stubLines(rowHeight: number) {
  stubRects((element) => {
    if (element.querySelector(":scope > ul[data-logs-count]") !== null) {
      return { x: 0, y: 0, width: 400, height: 100 }
    }
    if (element.tagName !== "LI" || !element.hasAttribute("data-key")) {
      return undefined
    }
    const list = element.parentElement as HTMLElement
    const index = Array.from(list.children).indexOf(element)
    return {
      x: 0,
      y: index * rowHeight - (list.parentElement?.scrollTop ?? 0),
      width: 400,
      height: rowHeight,
    }
  })
}

function hiddenKeys(
  layout: LogAnchorLayout,
  frames: PreparedFrame[]
): string[] {
  const shown = new Set(layout.lines.map(({ entry }) => entry.key))
  const count = frames[0].fields[0].values.length
  return Array.from(document.querySelectorAll("li[data-key]"))
    .map((row) => (row as HTMLElement).dataset.key ?? "")
    .filter((key) => !shown.has(key))
    .slice(0, count)
}

afterEach(() => {
  vi.restoreAllMocks()
  Reflect.deleteProperty(Element.prototype, "scrollIntoView")
})

describe("log anchors", () => {
  it("publishes a line for each entry it drew", () => {
    const { store } = renderPublishing(LogsPanel, panelOf("logs"), frames)
    const published = store.current()
    expect(published?.type).toBe("logs")
    const layout = published?.layout as LogAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.lines.map(({ entry }) => entry.message).sort()).toEqual([
      "first",
      "second",
      "third",
    ])
    expect(layout.resolve?.(clip(layout.lines[0].entry.key))).toBe(
      layout.lines[0].entry.key
    )
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view } = renderPublishing(LogsPanel, panelOf("logs"), frames)
    view.unmount()
    expect(store.current()).toBeNull()
  })

  it("culls lines scrolled out of the panel and points a pin on one at the bottom edge", () => {
    stubLines(50)
    const { store } = renderPublishing(LogsPanel, panelOf("logs"), lines(6))
    const layout = store.current()?.layout as LogAnchorLayout
    expect(layout.lines).toHaveLength(2)
    expect(layout.lines[1].rect).toEqual({
      x: 0,
      y: 50,
      width: 400,
      height: 50,
    })
    const hidden = hiddenKeys(layout, lines(6))
    expect(hidden).toHaveLength(4)
    expect(
      locate(layout, { kind: "log_line", pos: { x: 0, y: 0 }, id: hidden[0] })
    ).toEqual({ x: 0, y: 238, width: 400, height: 2 })
  })

  it("scrolls the panel's own scroller to a hidden line and never the page", () => {
    stubLines(50)
    const page = vi.fn()
    Element.prototype.scrollIntoView = page
    const { store, view } = renderPublishing(
      LogsPanel,
      panelOf("logs"),
      lines(6)
    )
    const layout = store.current()?.layout as LogAnchorLayout
    const last = view.container.querySelectorAll("li[data-key]")[5]
    layout.reveal?.((last as HTMLElement).dataset.key ?? "")
    expect(scroller(view.container).scrollTop).toBe(200)
    expect(page).not.toHaveBeenCalled()
  })

  it("scrolls a windowed panel's own scroller to a line by its offset", () => {
    stubLines(20)
    const page = vi.fn()
    Element.prototype.scrollIntoView = page
    const { store, view } = renderPublishing(
      LogsPanel,
      panelOf("logs"),
      lines(5000)
    )
    const element = scroller(view.container)
    const scrollTo = vi.fn()
    element.scrollTo = scrollTo
    const layout = store.current()?.layout as LogAnchorLayout
    const key = (
      view.container.querySelectorAll("li[data-key]")[3] as HTMLElement
    ).dataset.key
    layout.reveal?.(key ?? "")
    expect(scrollTo).toHaveBeenCalledTimes(1)
    expect(page).not.toHaveBeenCalled()
  })

  it("does no per-line work when a 5,000 line panel scrolls", () => {
    stubLines(20)
    const { store, view } = renderPublishing(
      LogsPanel,
      panelOf("logs"),
      lines(5000)
    )
    const first = store.current()?.layout
    const sets = vi.spyOn(Map.prototype, "set")
    const lengths: number[] = []
    const flatMap = Array.prototype.flatMap
    vi.spyOn(Array.prototype, "flatMap").mockImplementation(function (
      this: unknown[],
      ...args: Parameters<typeof flatMap>
    ) {
      lengths.push(this.length)
      return flatMap.apply(this, args)
    } as typeof Array.prototype.flatMap)
    const element = scroller(view.container)
    element.scrollTop = 400
    act(() => {
      element.dispatchEvent(new Event("scroll"))
    })
    expect(store.current()?.layout).not.toBe(first)
    expect(sets.mock.calls.length).toBeLessThan(1000)
    expect(Math.max(0, ...lengths)).toBeLessThan(1000)
  })
})
