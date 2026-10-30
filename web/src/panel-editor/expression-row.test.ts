import { describe, expect, it } from "vitest"

import type { ExpressionQuery } from "@/spec"

import { expressionSummary } from "./expression-row"

describe("expressionSummary", () => {
  const cases: [string, ExpressionQuery["expression"], string][] = [
    ["math", { type: "math", math: "$A / $B" }, "$A / $B"],
    ["reduce", { type: "reduce", input: "A", reducer: "max" }, "max of A"],
    [
      "resample",
      {
        type: "resample",
        input: "A",
        window: "5m",
        downsampler: "mean",
        upsampler: "pad",
      },
      "A every 5m, mean down and pad up",
    ],
    [
      "threshold",
      { type: "threshold", input: "A", evaluator: "gt", params: [5] },
      "A gt 5",
    ],
    [
      "range threshold",
      {
        type: "threshold",
        input: "A",
        evaluator: "withinRange",
        params: [1, 2],
      },
      "A withinRange 1 and 2",
    ],
  ]

  it.each(cases)("summarises a %s expression", (_name, expression, want) => {
    expect(expressionSummary(expression)).toBe(want)
  })
})
