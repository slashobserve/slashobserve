import { describe, expect, it } from "vitest"

import {
  annotationsFor,
  cardLeft,
  clusterFlags,
  linkParts,
  markerSpans,
  type MarkerSpan,
} from "./markers"
import type { Annotation } from "./queries"

const t0 = Date.parse("2026-11-01T10:00:00Z")
const minute = 60_000

function iso(ms: number): string {
  return new Date(ms).toISOString()
}

function annotation(over: Partial<Annotation> = {}): Annotation {
  return {
    id: over.id ?? "a",
    dashboard_id: null,
    panel_id: null,
    time_from: iso(t0),
    time_to: null,
    text: "deploy",
    tags: [],
    created_by: null,
    created_by_name: null,
    actor: null,
    created_at: iso(t0),
    ...over,
  }
}

const range = { fromMs: t0, toMs: t0 + 100 * minute, width: 1000 }

describe("annotationsFor", () => {
  const list = [
    annotation({ id: "global", tags: ["deploy"] }),
    annotation({ id: "empty", panel_id: "", tags: ["incident"] }),
    annotation({ id: "mine", panel_id: "rps", tags: ["deploy", "db"] }),
    annotation({ id: "other", panel_id: "latency", tags: ["deploy"] }),
  ]
  const ids = (out: Annotation[]) => out.map((a) => a.id)

  it("keeps global, unscoped and this panel's annotations", () => {
    expect(ids(annotationsFor(list, "rps", undefined))).toEqual([
      "global",
      "empty",
      "mine",
    ])
  })

  it("keeps annotations with any of the tags", () => {
    expect(ids(annotationsFor(list, "rps", ["db", "incident"]))).toEqual([
      "empty",
      "mine",
    ])
    expect(ids(annotationsFor(list, "rps", []))).toHaveLength(3)
  })
})

describe("markerSpans", () => {
  const xs = (spans: MarkerSpan[]) => spans.map((s) => [s.x, s.x2])

  it("places an instant inside the range", () => {
    expect(
      xs(markerSpans([annotation({ time_from: iso(t0 + 25 * minute) })], range))
    ).toEqual([[250, null]])
  })

  it("keeps an instant at either edge and drops one outside", () => {
    const list = [
      annotation({ id: "left", time_from: iso(t0) }),
      annotation({ id: "right", time_from: iso(t0 + 100 * minute) }),
      annotation({ id: "before", time_from: iso(t0 - 1) }),
      annotation({ id: "after", time_from: iso(t0 + 100 * minute + 1) }),
    ]
    expect(xs(markerSpans(list, range))).toEqual([
      [0, null],
      [1000, null],
    ])
  })

  it("clamps a range on the left, the right and both sides", () => {
    const left = annotation({
      time_from: iso(t0 - 10 * minute),
      time_to: iso(t0 + 10 * minute),
    })
    const right = annotation({
      time_from: iso(t0 + 90 * minute),
      time_to: iso(t0 + 200 * minute),
    })
    const both = annotation({
      time_from: iso(t0 - minute),
      time_to: iso(t0 + 101 * minute),
    })
    expect(xs(markerSpans([left], range))).toEqual([[0, 100]])
    expect(xs(markerSpans([right], range))).toEqual([[900, 1000]])
    expect(xs(markerSpans([both], range))).toEqual([[0, 1000]])
  })

  it("drops a range that ends before or starts after the window", () => {
    const list = [
      annotation({ time_from: iso(t0 - 20 * minute), time_to: iso(t0 - 1) }),
      annotation({
        time_from: iso(t0 + 101 * minute),
        time_to: iso(t0 + 102 * minute),
      }),
    ]
    expect(markerSpans(list, range)).toEqual([])
  })

  it("skips an invalid time_from", () => {
    expect(markerSpans([annotation({ time_from: "soon" })], range)).toEqual([])
  })

  it("draws nothing at zero width or an empty range", () => {
    expect(markerSpans([annotation()], { ...range, width: 0 })).toEqual([])
    expect(markerSpans([annotation()], { ...range, toMs: t0 })).toEqual([])
  })

  it("sorts by position", () => {
    const list = [
      annotation({ id: "late", time_from: iso(t0 + 50 * minute) }),
      annotation({ id: "early", time_from: iso(t0 + 10 * minute) }),
    ]
    expect(markerSpans(list, range).map((s) => s.annotation.id)).toEqual([
      "early",
      "late",
    ])
  })
})

describe("clusterFlags", () => {
  function spanAt(id: string, x: number, ms: number): MarkerSpan {
    return { annotation: annotation({ id, time_from: iso(ms) }), x, x2: null }
  }

  it("merges markers 8 px apart and lists them newest first", () => {
    const flags = clusterFlags([spanAt("a", 100, t0), spanAt("b", 108, t0 + 1)])
    expect(flags).toHaveLength(1)
    expect(flags[0]?.x).toBe(100)
    expect(flags[0]?.annotations.map((a) => a.id)).toEqual(["b", "a"])
  })

  it("splits markers 9 px apart", () => {
    const flags = clusterFlags([spanAt("a", 100, t0), spanAt("b", 109, t0 + 1)])
    expect(flags.map((f) => f.x)).toEqual([100, 109])
  })
})

describe("linkParts", () => {
  const origin = "https://observe.example.com"

  it("links an https address", () => {
    expect(linkParts("CI https://ci.example.com/run/1", origin)).toEqual([
      { text: "CI " },
      {
        text: "https://ci.example.com/run/1",
        href: "https://ci.example.com/run/1",
      },
    ])
  })

  it("links a same-origin path and keeps a trailing full stop outside", () => {
    expect(linkParts("Pinned /findings/abc.", origin)).toEqual([
      { text: "Pinned " },
      { text: "/findings/abc", href: "/findings/abc" },
      { text: "." },
    ])
  })

  it("leaves protocol-relative, javascript and mailto addresses as text", () => {
    const text = "//evil.example javascript:alert(1) mailto:a@b.c"
    expect(linkParts(text, origin)).toEqual([{ text }])
  })
})

describe("cardLeft", () => {
  it("centres the card on the flag", () => {
    expect(cardLeft(500, 280, 1000)).toBe(360)
  })

  it("clamps the card at both edges", () => {
    expect(cardLeft(10, 280, 1000)).toBe(0)
    expect(cardLeft(990, 280, 1000)).toBe(720)
    expect(cardLeft(50, 280, 200)).toBe(0)
  })
})
