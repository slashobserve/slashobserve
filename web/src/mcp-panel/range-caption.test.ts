import { describe, expect, it } from "vitest"

import recorded from "./fixtures/show-panel-result.json"
import type { PanelPayload } from "./payload"
import { rangeCaption } from "./range-caption"

const payload = recorded.structuredContent as unknown as PanelPayload

describe("rangeCaption", () => {
  it("shows the resolved range in the given zone", () => {
    expect(rangeCaption(payload, "UTC")).toBe(
      "2026-11-25 14:31:43 to 2026-11-25 15:31:43 UTC"
    )
    expect(rangeCaption(payload, "Asia/Kolkata")).toBe(
      "2026-11-25 20:01:43 to 2026-11-25 21:01:43 Asia/Kolkata"
    )
  })

  it("falls back to the requested range when the times do not parse", () => {
    const odd = {
      ...payload,
      data: { ...payload.data, meta: { ...payload.data.meta, from: "x" } },
    } as PanelPayload
    expect(rangeCaption(odd, "UTC")).toBe("now-1h to now")
  })
})
