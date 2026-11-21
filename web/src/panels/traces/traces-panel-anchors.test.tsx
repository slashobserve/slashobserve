import { describe, expect, it, vi } from "vitest"

import { renderInStore } from "../anchor-harness"
import { createFieldResolver } from "../field-config"
import type { PreparedField, PreparedFrame } from "../transforms"
import type { DataPanel } from "@/spec"
import type { SpanAnchorLayout } from "./anchors"
import { spanFilters } from "./span-filters"
import { TraceView } from "./traces-panel"
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
    expect(layout.elsewhere).toEqual([])
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view: mounted } = view()
    mounted.unmount()
    expect(store.current()).toBeNull()
  })

  it("publishes a reveal that scrolls the span's row into view", () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    const { store, view: mounted } = view()
    const layout = store.current()?.layout as SpanAnchorLayout
    layout.reveal?.("b")
    expect(scroll).toHaveBeenCalledTimes(1)
    expect((scroll.mock.contexts[0] as HTMLElement).dataset.spanId).toBe("b")
    expect(mounted.container.contains(scroll.mock.contexts[0])).toBe(true)
  })
})
