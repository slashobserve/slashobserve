import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { Alert } from "@/components/ui/alert"

import { NewerVersionNotice, newerVersionTitle } from "./newer-version-notice"

describe("newerVersionTitle", () => {
  it("names who saved", () => {
    expect(newerVersionTitle(7, "Ada")).toBe(
      "Ada saved version 7 while you were editing."
    )
  })

  it("falls back to the version alone", () => {
    expect(newerVersionTitle(7, null)).toBe(
      "Version 7 was saved while you were editing."
    )
  })
})

describe("NewerVersionNotice", () => {
  it("shows both values of a field that overlaps", () => {
    render(
      <Alert>
        <NewerVersionNotice
          version={2}
          author="Ada"
          overlaps={[{ key: "title", label: "title", theirs: "A", yours: "B" }]}
        />
      </Alert>
    )
    expect(
      screen.getByText('title: version 2 has "A", yours is "B"')
    ).toBeInTheDocument()
  })

  it("says nothing overlaps when it does not", () => {
    render(
      <Alert>
        <NewerVersionNotice version={2} author={null} overlaps={[]} />
      </Alert>
    )
    expect(screen.getByText(/Nothing you changed overlaps/)).toBeInTheDocument()
  })
})
