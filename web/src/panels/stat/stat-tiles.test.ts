import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { WireField } from "@/panels/frames"
import type { PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import { statTiles } from "./stat-tiles"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    type: "stat",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

const time: WireField = {
  name: "Time",
  type: "time",
  values: [1000, 2000, 3000],
}

function frame(
  values: (number | null)[],
  extra: Partial<PreparedFrame> = {},
  name = "Value"
): PreparedFrame {
  return {
    ref: "A",
    fields: [time, { name, type: "number", values }],
    ...extra,
  }
}

function tiles(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  reduce: Parameters<typeof statTiles>[2] = { reduce: "lastNotNull" }
) {
  const p = panel(extra)
  return statTiles(frames, createFieldResolver(p), reduce, p.unit)
}

describe("statTiles", () => {
  it("reduces each field with the chosen reducer", () => {
    const frames = [frame([4, 9, 2])]
    expect(tiles(frames, {}, { reduce: "max" })[0].value).toBe(9)
    expect(tiles(frames, {}, { reduce: "mean" })[0].value).toBe(5)
    expect(tiles(frames, {}, { reduce: "last" })[0].value).toBe(2)
  })

  it("makes one tile per number field, skipping hidden ones", () => {
    const two: PreparedFrame = {
      ref: "A",
      fields: [
        time,
        { name: "a", type: "number", values: [1, 2, 3] },
        { name: "b", type: "number", values: [4, 5, 6] },
        { name: "label", type: "string", values: ["x", "y", "z"] },
      ],
    }
    expect(tiles([two]).map((tile) => tile.fieldName)).toEqual(["a", "b"])
    const hidden = tiles([two], {
      overrides: [{ match: { field: "b" }, hidden: true }],
    } as Partial<DataPanel>)
    expect(hidden.map((tile) => tile.fieldName)).toEqual(["a"])
  })

  it("shares one decimal count across tiles", () => {
    const result = tiles([frame([2.4]), { ...frame([2.45]), ref: "B" }], {
      unit: "short",
    })
    expect(result.map((tile) => tile.text)).toEqual(["2.40", "2.45"])
  })

  it("prints a mapped value as its text and color", () => {
    const [tile] = tiles([frame([1])], {
      mappings: [{ value: "1", text: "Up", color: "ok" }],
    } as Partial<DataPanel>)
    expect(tile.text).toBe("Up")
    expect(tile.color).toBe("var(--threshold-ok)")
  })

  it("colors by threshold", () => {
    const [tile] = tiles([frame([7])], {
      thresholds: [{ color: "ok" }, { value: 5, color: "critical" }],
    })
    expect(tile.threshold.level).toBe("critical")
    expect(tile.color).toBe("var(--threshold-critical)")
  })

  it("shows noValue for a field with no numbers", () => {
    const [tile] = tiles([frame([null, null, null])], { noValue: "n/a" })
    expect(tile.value).toBeNull()
    expect(tile.text).toBe("n/a")
  })

  it.each([
    ["reqps", 256, "256", "req/s"],
    ["bytes", 1536, "1.5", "KiB"],
    ["percent", 42, "42%", ""],
    ["prefix:$", 3, "$3", ""],
    ["short", 7, "7", ""],
  ])("splits the %s unit from the amount", (unit, value, amount, suffix) => {
    const [tile] = tiles([frame([value])], { unit })
    expect([tile.amount, tile.unit]).toEqual([amount, suffix])
  })

  it("keeps a mapped text whole", () => {
    const [tile] = tiles([frame([1])], {
      mappings: [{ value: "1", text: "3 pods", color: "ok" }],
    } as Partial<DataPanel>)
    expect([tile.amount, tile.unit]).toEqual(["3 pods", ""])
  })

  it("keeps the time and value pairs of a sparkline, dropping gaps", () => {
    const [tile] = tiles([frame([1, null, 3])])
    expect(tile.points).toEqual([
      [1000, 1],
      [3000, 3],
    ])
  })

  it("keeps same-named series on distinct keys", () => {
    const keys = tiles([frame([1]), frame([2])]).map((tile) => tile.key)
    expect(new Set(keys).size).toBe(2)
  })

  it("names a tile from the display name template", () => {
    const [tile] = tiles([frame([1], { labels: { pod: "web-0" } })], {
      displayName: "pod ${__field.labels.pod}",
    })
    expect(tile.name).toBe("pod web-0")
  })
})
