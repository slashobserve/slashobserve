import { act, render } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AnchorLayoutProvider, createAnchorLayoutStore } from "../anchor-layout"
import { renderInStore, stubRects } from "../anchor-harness"
import { createFieldResolver } from "../field-config"
import type { PreparedField, PreparedFrame } from "../transforms"
import type { DataPanel } from "@/spec"
import { locate, type SpanAnchorLayout } from "./anchors"
import { foldGroups, foldThreshold } from "./fold"
import { spanFilters } from "./span-filters"
import { TraceView, type TraceViewProps } from "./traces-panel"
import { traceModel } from "./trace-model"

function field(
  name: string,
  type: PreparedField["type"],
  values: unknown[]
): PreparedField {
  return { name, type, values }
}

const frames: PreparedFrame[] = [
  {
    ref: "A",
    fields: [
      field("traceID", "string", ["t1", "t1", "t1"]),
      field("spanID", "string", ["a", "b", "c"]),
      field("parentSpanID", "string", ["", "a", "a"]),
      field("operationName", "string", ["GET /", "db", "cache"]),
      field("serviceName", "string", ["web", "db", "cache"]),
      field("startTime", "time", [1000, 1010, 1060]),
      field("duration", "number", [100, 40, 20]),
    ],
  },
]

const model = traceModel(
  frames,
  createFieldResolver({
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "traces",
    datasource: "ds",
    queries: [{ query: "q" }],
  } as DataPanel)
)

function view() {
  if (model.reason !== null) throw new Error(model.reason)
  return renderInStore(
    <TraceView
      model={model}
      filters={spanFilters({})}
      zone="UTC"
      width={400}
      height={240}
      attributes="inline"
    />
  )
}

describe("trace anchors", () => {
  it("publishes a span for each row it drew", () => {
    const { store } = view()
    const published = store.current()
    expect(published?.type).toBe("traces")
    const layout = published?.layout as SpanAnchorLayout
    expect(layout.spans.map(({ span }) => span.id)).toEqual(["a", "b", "c"])
    expect(layout.resolve?.("b")).toBe("b")
    expect(layout.resolve?.("nope")).toBeUndefined()
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view: mounted } = view()
    mounted.unmount()
    expect(store.current()).toBeNull()
  })
})

const resolver = createFieldResolver({
  id: "p1",
  title: "Panel",
  gridPos: { x: 0, y: 0, w: 12, h: 8 },
  type: "traces",
  datasource: "ds",
  queries: [{ query: "q" }],
} as DataPanel)

function crowdModel(count: number) {
  const ids = ["r", ...Array.from({ length: count }, (_, index) => `c${index}`)]
  const made = traceModel(
    [
      {
        ref: "A",
        fields: [
          field(
            "traceID",
            "string",
            ids.map(() => "t1")
          ),
          field("spanID", "string", ids),
          field(
            "parentSpanID",
            "string",
            ids.map((id) => (id === "r" ? "" : "r"))
          ),
          field(
            "operationName",
            "string",
            ids.map((id) => (id === "r" ? "root" : "work"))
          ),
          field(
            "serviceName",
            "string",
            ids.map(() => "web")
          ),
          field(
            "startTime",
            "time",
            ids.map((_, index) => 1000 + index)
          ),
          field(
            "duration",
            "number",
            ids.map(() => 10)
          ),
        ],
      },
    ],
    resolver
  )
  if (made.reason !== null) throw new Error(made.reason)
  return made
}

function mount(count: number, extra: Partial<TraceViewProps> = {}) {
  const model = crowdModel(count)
  const store = createAnchorLayoutStore()
  const tree = (more: Partial<TraceViewProps>) => (
    <AnchorLayoutProvider value={store}>
      <TraceView
        model={model}
        filters={spanFilters({})}
        zone="UTC"
        width={400}
        height={240}
        attributes="inline"
        {...extra}
        {...more}
      />
    </AnchorLayoutProvider>
  )
  const view = render(tree({}))
  return {
    store,
    view,
    model,
    layout: () => store.current()?.layout as SpanAnchorLayout,
    update: (more: Partial<TraceViewProps>) => view.rerender(tree(more)),
  }
}

function stubTree(detail = 0) {
  stubRects((element) => {
    if (element.querySelector(":scope > ul[role=tree]") !== null) {
      return { x: 0, y: 0, width: 400, height: 240 }
    }
    if (element.className.toString().startsWith("sticky")) {
      return { x: 0, y: 0, width: 400, height: 25 }
    }
    if (element.className.toString().includes("border-t border-border px-2")) {
      return { x: 0, y: 0, width: 400, height: detail }
    }
    const item = element.tagName === "LI" ? element : element.parentElement
    const tree = item?.parentElement
    if (item?.tagName !== "LI" || tree?.getAttribute("role") !== "tree") {
      return undefined
    }
    if (item !== element && element !== item.firstElementChild) return undefined
    const scroller = tree.parentElement as HTMLElement
    return {
      x: 0,
      y: 25 + Array.from(tree.children).indexOf(item) * 32 - scroller.scrollTop,
      width: 400,
      height: 32,
    }
  })
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  Reflect.deleteProperty(Element.prototype, "scrollIntoView")
})

const scrollerOf = (container: HTMLElement) =>
  container.querySelector("ul[role=tree]")?.parentElement as HTMLElement

describe("trace anchors with measured rows", () => {
  it("keeps the rows below the ruler and resolves one scrolled out of view", () => {
    stubTree()
    const { layout } = mount(20)
    expect(layout().spans.map(({ span }) => span.id)).toEqual([
      "r",
      "c0",
      "c1",
      "c2",
      "c3",
      "c4",
      "c5",
    ])
    expect(layout().spans[1].rect).toEqual({
      x: 0,
      y: 57,
      width: 400,
      height: 32,
    })
    expect(layout().resolve?.("c10")).toBe("c10")
    expect(
      locate(layout(), { kind: "span", pos: { x: 0, y: 0 }, id: "c10" })
    ).toEqual({ x: 0, y: 238, width: 400, height: 2 })
  })

  it("scrolls the trace's own scroller to a hidden span and never the page", () => {
    stubTree()
    const page = vi.fn()
    Element.prototype.scrollIntoView = page
    const { layout, view } = mount(20)
    layout().reveal?.("c10")
    expect(scrollerOf(view.container).scrollTop).toBe(232)
    expect(page).not.toHaveBeenCalled()
  })

  it("counts the open span's detail pane above a span below it", () => {
    stubTree(60)
    class Immediate {
      callback: ResizeObserverCallback
      constructor(callback: ResizeObserverCallback) {
        this.callback = callback
      }
      observe() {
        this.callback(
          [{ contentRect: { width: 400, height: 60 } } as ResizeObserverEntry],
          this as unknown as ResizeObserver
        )
      }
      unobserve() {}
      disconnect() {}
    }
    vi.stubGlobal("ResizeObserver", Immediate)
    Element.prototype.scrollIntoView = vi.fn()
    const { layout, view } = mount(20, {
      openId: "c0",
      onOpenIdChange: () => undefined,
    })
    layout().reveal?.("c10")
    expect(scrollerOf(view.container).scrollTop).toBe(292)
  })

  it("expands a collapsed ancestor, then scrolls to the span it hid", () => {
    stubTree()
    const onCollapsedChange = vi.fn()
    const { layout, update, view } = mount(20, {
      collapsed: new Set(["r"]),
      onCollapsedChange,
    })
    expect(layout().spans.map(({ span }) => span.id)).toEqual(["r"])
    expect(layout().resolve?.("c10")).toBe("c10")
    expect(
      locate(layout(), { kind: "span", pos: { x: 0, y: 0 }, id: "c10" })
    ).toEqual({ x: 0, y: 238, width: 400, height: 2 })
    act(() => layout().reveal?.("c10"))
    expect(onCollapsedChange).toHaveBeenCalledWith(new Set())
    update({ collapsed: new Set(), onCollapsedChange })
    expect(scrollerOf(view.container).scrollTop).toBe(232)
  })

  it("opens the fold group that holds a span, then scrolls to it", () => {
    stubTree()
    const model = crowdModel(foldThreshold + 5)
    const { layout, view } = mount(foldThreshold + 5, {
      fold: foldGroups(model.spans, foldThreshold),
    })
    expect(layout().spans.map(({ span }) => span.id)).toEqual(["r"])
    expect(layout().resolve?.("c15")).toBe("c15")
    act(() => layout().reveal?.("c15"))
    expect(view.container.querySelector('[data-span-id="c15"]')).not.toBeNull()
    expect(scrollerOf(view.container).scrollTop).toBeGreaterThan(0)
  })
})
