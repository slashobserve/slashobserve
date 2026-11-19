import { describe, expect, it } from "vitest"

import { withoutEditor } from "./editor-entry"

describe("withoutEditor", () => {
  it("drops the panel and edit keys and keeps the rest of the address", () => {
    expect(
      withoutEditor("/d/abc/shop?from=now-6h&panel=p_requests&edit=query#x")
    ).toBe("/d/abc/shop?from=now-6h#x")
  })

  it("leaves an address without an editor unchanged", () => {
    expect(withoutEditor("/d/abc/shop?var-env=prod")).toBe(
      "/d/abc/shop?var-env=prod"
    )
  })
})
