import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "../transforms"
import {
  bodyWidth,
  candleOptions,
  directions,
  findCandleField,
  minSpacing,
  noCandlesReason,
  noVolumeReason,
  rowColors,
  selectCandles,
  volumeHeight,
  volumeShare,
} from "./candles"

function frame(names: string[]): PreparedFrame {
  return {
    ref: "A",
    fields: [
      { name: "time", type: "time", values: [1, 2] },
      ...names.map((name) => ({
        name,
        type: "number" as const,
        values: [1, 2],
      })),
    ],
  } as PreparedFrame
}

const all = ["Open", "HIGH", "low", "Close", "Volume"]

describe("findCandleField", () => {
  it("matches the default names case-insensitively", () => {
    expect(findCandleField(frame(all), "high", undefined)?.name).toBe("HIGH")
  })

  it("prefers an explicit name", () => {
    const found = findCandleField(frame(["open", "o"]), "open", "o")
    expect(found?.name).toBe("o")
  })

  it("falls back to the default name when the explicit one is absent", () => {
    const found = findCandleField(frame(["open"]), "open", "missing")
    expect(found?.name).toBe("open")
  })

  it("ignores fields that are not numbers", () => {
    const text = {
      ref: "A",
      fields: [{ name: "open", type: "string", values: ["a"] }],
    } as PreparedFrame
    expect(findCandleField(text, "open", undefined)).toBeUndefined()
  })
})

describe("selectCandles", () => {
  it("lists price fields in order, then volume, for candles and volume", () => {
    const selection = selectCandles([frame(all)], candleOptions({}))
    expect(selection.reason).toBeNull()
    expect(
      "columns" in selection && selection.columns.map((c) => c.role)
    ).toEqual(["open", "high", "low", "close", "volume"])
    expect(
      "frame" in selection && selection.frame.fields.map((f) => f.name)
    ).toEqual(["time", "Open", "HIGH", "low", "Close", "Volume"])
  })

  it("leaves volume out in candles mode", () => {
    const selection = selectCandles(
      [frame(all)],
      candleOptions({ mode: "candles" })
    )
    expect(
      "columns" in selection && selection.columns.map((c) => c.role)
    ).toEqual(["open", "high", "low", "close"])
  })

  it("draws candles without volume when the frame has none", () => {
    const selection = selectCandles(
      [frame(["open", "high", "low", "close"])],
      candleOptions({})
    )
    expect(
      "columns" in selection && selection.columns.map((c) => c.role)
    ).toEqual(["open", "high", "low", "close"])
  })

  it("never draws other fields", () => {
    const selection = selectCandles(
      [frame([...all, "vwap"])],
      candleOptions({})
    )
    expect(
      "frame" in selection &&
        selection.frame.fields.some((f) => f.name === "vwap")
    ).toBe(false)
  })

  it("explains a frame missing one of open, high, low or close", () => {
    const selection = selectCandles(
      [frame(["open", "high", "low"])],
      candleOptions({})
    )
    expect(selection.reason).toBe(noCandlesReason)
    expect(noCandlesReason).toBe(
      "A candlestick needs open, high, low and close fields."
    )
  })

  it("uses the first frame that qualifies", () => {
    const selection = selectCandles(
      [frame(["open"]), frame(all)],
      candleOptions({})
    )
    expect(selection.reason).toBeNull()
  })

  it("rejects a frame without a time field", () => {
    const noTime = {
      ref: "A",
      fields: frame(all).fields.slice(1),
    } as PreparedFrame
    expect(selectCandles([noTime], candleOptions({})).reason).toBe(
      noCandlesReason
    )
  })

  it("needs only volume in volume mode, with prices when present", () => {
    const options = candleOptions({ mode: "volume" })
    const onlyVolume = selectCandles([frame(["volume"])], options)
    expect(
      "columns" in onlyVolume && onlyVolume.columns.map((c) => c.role)
    ).toEqual(["volume"])
    const withPrices = selectCandles([frame(all)], options)
    expect(
      "columns" in withPrices && withPrices.columns.map((c) => c.role)
    ).toEqual(["open", "high", "low", "close", "volume"])
    expect(selectCandles([frame(["open"])], options).reason).toBe(
      noVolumeReason
    )
  })
})

describe("candleOptions", () => {
  it("reads the contract defaults", () => {
    expect(candleOptions({})).toMatchObject({
      mode: "candles+volume",
      candleStyle: "candles",
      colorStrategy: "open-close",
      upColor: "ok",
      downColor: "critical",
    })
  })

  it("keeps explicit field names", () => {
    expect(candleOptions({ open: "o", volume: "v" }).names).toEqual({
      open: "o",
      volume: "v",
    })
  })
})

describe("rowColors", () => {
  const colors = { upColor: "ok", downColor: "#112233" }

  it("gives each row the color of its candle direction", () => {
    expect(
      rowColors([10, 10, 12], [11, 9, 12], 3, {
        ...colors,
        colorStrategy: "open-close",
      })
    ).toEqual(["var(--threshold-ok)", "#112233", "var(--threshold-ok)"])
  })

  it("follows the color strategy", () => {
    expect(
      rowColors([5, 5, 5], [6, 7, 6], 3, {
        ...colors,
        colorStrategy: "close-close",
      })
    ).toEqual(["var(--threshold-ok)", "var(--threshold-ok)", "#112233"])
  })

  it("paints a row without a direction, or without prices, in the up color", () => {
    const up = "var(--threshold-ok)"
    expect(
      rowColors([5, 5], [6, null], 2, {
        ...colors,
        colorStrategy: "open-close",
      })
    ).toEqual([up, up])
    expect(
      rowColors([], [], 2, { ...colors, colorStrategy: "open-close" })
    ).toEqual([up, up])
  })
})

describe("directions", () => {
  const open = [10, 10, 12, 11]
  const close = [11, 9, 12, 13]

  it("compares close with open", () => {
    expect(directions(open, close, "open-close")).toEqual([
      "up",
      "down",
      "up",
      "up",
    ])
  })

  it("compares close with the previous close", () => {
    expect(directions(open, close, "close-close")).toEqual([
      "up",
      "down",
      "up",
      "up",
    ])
    expect(directions([5, 5, 5], [6, 7, 6], "close-close")).toEqual([
      "up",
      "up",
      "down",
    ])
    expect(directions([5, 5, 5], [6, 7, 6], "open-close")).toEqual([
      "up",
      "up",
      "up",
    ])
  })

  it("skips a row with no close and keeps the last close for the next", () => {
    expect(directions([5, 5, 5], [6, null, 5], "close-close")).toEqual([
      "up",
      null,
      "down",
    ])
  })

  it("has no direction when the reference is missing", () => {
    expect(directions([null], [3], "open-close")).toEqual([null])
  })
})

describe("geometry", () => {
  it("finds the smallest positive gap", () => {
    expect(minSpacing([0, 10, 15, 40])).toBe(5)
    expect(minSpacing([4])).toBe(Infinity)
  })

  it("keeps a body between one pixel and 64 pixels", () => {
    expect(bodyWidth(10, 1)).toBeCloseTo(7)
    expect(bodyWidth(0.5, 2)).toBe(2)
    expect(bodyWidth(Infinity, 2)).toBe(128)
  })

  it("scales volume into the bottom fifth", () => {
    expect(volumeShare).toBe(0.2)
    expect(volumeHeight(50, 100, 500)).toBe(50)
    expect(volumeHeight(100, 100, 500)).toBe(100)
    expect(volumeHeight(null, 100, 500)).toBe(0)
    expect(volumeHeight(-1, 100, 500)).toBe(0)
    expect(volumeHeight(5, 0, 500)).toBe(0)
  })
})
