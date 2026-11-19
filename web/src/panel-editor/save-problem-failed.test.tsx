import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { SaveProblem } from "./save-problem"

describe("SaveProblem", () => {
  it("starts a failed save's message with a capital and ends it with a full stop", () => {
    render(
      <SaveProblem
        problem={{
          kind: "failed",
          message: "your role does not allow this; it needs Edit on Payments",
        }}
        panelId="p1"
        onRetry={() => {}}
      />
    )
    expect(
      screen.getByText(
        "Your role does not allow this; it needs Edit on Payments."
      )
    ).toBeVisible()
  })
})
