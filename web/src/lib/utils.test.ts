import { describe, expect, it } from "vitest"

import { cn } from "@/lib/utils"

describe("cn", () => {
  it("lets a later radius utility replace the custom radii", () => {
    expect(cn("rounded-control", "rounded-full")).toBe("rounded-full")
    expect(cn("rounded-card", "rounded-full")).toBe("rounded-full")
    expect(cn("rounded-full", "rounded-control")).toBe("rounded-control")
  })

  it("keeps unrelated classes", () => {
    expect(cn("rounded-control px-2", "px-3")).toBe("rounded-control px-3")
  })
})
