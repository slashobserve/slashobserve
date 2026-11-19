import { describe, expect, it } from "vitest"

import { seriesCapLabel } from "./series-cap"

describe("seriesCapLabel", () => {
  it("names how many series are shown of how many", () => {
    expect(seriesCapLabel({ series_shown: 50, series_total: 412 })?.text).toBe(
      "Top 50 of 412 series"
    )
  })

  it("says nothing when the cap dropped nothing", () => {
    expect(seriesCapLabel({})).toBeNull()
    expect(seriesCapLabel(undefined)).toBeNull()
    expect(seriesCapLabel({ series_shown: 5, series_total: 5 })).toBeNull()
  })
})
