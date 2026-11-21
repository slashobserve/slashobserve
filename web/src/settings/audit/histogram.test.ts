import { describe, expect, test } from "vitest"

import { barHeight, stepText } from "./histogram"

describe("stepText", () => {
  test("names the largest whole unit", () => {
    expect(stepText(86_400_000)).toBe("day")
    expect(stepText(7_200_000)).toBe("2 hours")
    expect(stepText(1_800_000)).toBe("30 minutes")
    expect(stepText(90_000)).toBe("90 seconds")
    expect(stepText(250)).toBe("250 ms")
  })
})

describe("barHeight", () => {
  test("keeps an empty bucket visible and scales the rest", () => {
    expect(barHeight(0, 10)).toBe(1)
    expect(barHeight(5, 10)).toBe("50%")
    expect(barHeight(10, 10)).toBe("100%")
  })
})
