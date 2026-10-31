import { describe, expect, it } from "vitest"

import { candlestickLegend } from "./candlestick-legend"

describe("candlestickLegend", () => {
  it.each([
    ["a plain list legend", {}, "upDown"],
    ["a bottom list legend", { legend: "bottom" }, "upDown"],
    ["an explicit list mode", { legendMode: "list" }, "upDown"],
    ["an empty values list", { legendValues: [] }, "upDown"],
    ["table mode", { legendMode: "table" }, "series"],
    ["legend values", { legendValues: ["mean", "p95"] }, "series"],
    [
      "legend values on the right",
      { legend: "right", legendValues: ["last"] },
      "series",
    ],
    [
      "table mode on the right",
      { legend: "right", legendMode: "table" },
      "series",
    ],
    [
      "a list mode that still has values",
      { legendMode: "list", legendValues: ["max"] },
      "series",
    ],
    ["a hidden legend", { legend: "hidden" }, "none"],
    [
      "a hidden legend with table mode and values",
      { legend: "hidden", legendMode: "table", legendValues: ["mean"] },
      "none",
    ],
  ])("chooses the right legend for %s", (_name, display, expected) => {
    expect(candlestickLegend(display)).toBe(expected)
  })
})
