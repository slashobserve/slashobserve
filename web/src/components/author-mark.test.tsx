import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { AuthorMarkModel } from "@/lib/author"

import { AuthorMark, AuthorStack } from "./author-mark"

const person: AuthorMarkModel = { kind: "person", text: "PR", tint: 3 }
const agent: AuthorMarkModel = {
  kind: "observe-agent",
  text: "/",
  tint: null,
}

function entries(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    key: `a${i}`,
    mark: person,
    label: `Author ${i}`,
  }))
}

describe("AuthorStack", () => {
  it("exposes one image named by the label", () => {
    render(<AuthorStack marks={entries(3)} label="Worked on by Priya, Ishan" />)
    expect(
      screen.getAllByRole("img", { name: "Worked on by Priya, Ishan" })
    ).toHaveLength(1)
  })

  it("counts the marks beyond the maximum", () => {
    render(<AuthorStack marks={entries(6)} label="Six authors" />)
    expect(screen.getByText("+1")).toBeInTheDocument()
  })

  it("shows no count when every mark fits", () => {
    render(<AuthorStack marks={entries(5)} label="Five authors" />)
    expect(screen.queryByText(/^\+/)).toBeNull()
  })
})

describe("AuthorMark", () => {
  it("draws the slash for the built-in agent", () => {
    render(<AuthorMark mark={agent} />)
    expect(screen.getByText("/")).toBeInTheDocument()
  })

  it("is hidden from assistive technology", () => {
    render(<AuthorMark mark={person} size={20} />)
    expect(screen.getByText("PR")).toHaveAttribute("aria-hidden", "true")
  })
})
