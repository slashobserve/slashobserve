import { render } from "@testing-library/react"
import { describe, expect, test } from "vitest"

import type { ProblemGroup } from "./health-labels"
import { KindSummary } from "./kind-summary"

function group(fields: Partial<ProblemGroup> = {}): ProblemGroup {
  return {
    kind: "query_failed",
    message: null,
    subjects: ["Checkout errors by message"],
    ...fields,
  }
}

describe("KindSummary", () => {
  test("keeps the separator on the kind label so a wrapped summary never starts with it", () => {
    const { container } = render(<KindSummary group={group()} />)
    const [label, summary] = Array.from(container.children)
    expect(label?.textContent).toBe("Query fails·")
    expect(summary?.textContent).toBe("Checkout errors by message")
  })

  test("hides the separator from assistive technology", () => {
    const { container } = render(<KindSummary group={group()} />)
    expect(container.querySelector("span[aria-hidden=true]")?.textContent).toBe(
      "·"
    )
  })

  test("counts the subjects when there are several", () => {
    const { container } = render(
      <KindSummary group={group({ subjects: ["A", "B"] })} />
    )
    expect(container.children[1]?.textContent).toBe("2 panels")
  })

  test("shows no separator without subjects", () => {
    const { container } = render(
      <KindSummary group={group({ subjects: [] })} />
    )
    expect(container.children).toHaveLength(1)
    expect(container.textContent).toBe("Query fails")
  })
})
