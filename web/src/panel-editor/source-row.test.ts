import { describe, expect, it } from "vitest"

import { sourceSummary } from "./source-row"

describe("sourceSummary", () => {
  it("names the source panel", () => {
    expect(sourceSummary({ panel: "p1" }, "Requests")).toBe(
      "Results of Requests"
    )
  })

  it("says when the source's transforms apply", () => {
    expect(
      sourceSummary({ panel: "p1", withTransforms: true }, "Requests")
    ).toBe("Results of Requests, after its transforms")
  })
})
