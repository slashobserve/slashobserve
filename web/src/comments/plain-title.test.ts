import { describe, expect, it } from "vitest"

import { plainTitle } from "./plain-title"

const id = "00000000-0000-0000-0000-000000000000"

describe("plainTitle", () => {
  it("shows a mention as the name it was written with", () => {
    expect(
      plainTitle(`Checkout is slow @[Ghost](user:${id}) please look`)
    ).toBe("Checkout is slow @Ghost please look")
  })

  it("keeps the agent handle and leaves ordinary text alone", () => {
    expect(plainTitle("Ask @observe about 5xx")).toBe("Ask @observe about 5xx")
    expect(plainTitle("p95 latency")).toBe("p95 latency")
  })

  it("shows every mention of a repeated one", () => {
    expect(plainTitle(`@[A](user:${id}) @[A](user:${id})`)).toBe("@A @A")
  })
})
