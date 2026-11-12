import { describe, expect, test } from "vitest"

import { foldedFor } from "./use-folded"

describe("foldedFor", () => {
  test.each([
    [320, true],
    [1000, true],
    [1199, true],
    [1200, false],
    [1440, false],
  ])("a %d px window folded is %s", (width, folded) => {
    expect(foldedFor(width)).toBe(folded)
  })
})
