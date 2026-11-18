import { render } from "@testing-library/react"
import { beforeEach, describe, expect, it } from "vitest"

import {
  SubNavSkeleton,
  storeRows,
  storedRows,
  useReservedRows,
} from "./sub-nav"

function Reserved({ pending, count }: { pending: boolean; count: number }) {
  const rows = useReservedRows("logs", pending, count)
  return <p data-testid="rows">{rows}</p>
}

describe("reserved sidebar rows", () => {
  beforeEach(() => localStorage.clear())

  it("starts with no rows for a list never seen", () => {
    expect(storedRows("logs")).toBe(0)
  })

  it("remembers the rows of a loaded list for the next page load", () => {
    const { unmount } = render(<Reserved pending={false} count={4} />)
    unmount()
    expect(storedRows("logs")).toBe(4)
    const next = render(<Reserved pending count={0} />)
    expect(next.getByTestId("rows").textContent).toBe("4")
  })

  it("keeps the remembered rows while the list is pending", () => {
    storeRows("logs", 3)
    render(<Reserved pending count={0} />)
    expect(storedRows("logs")).toBe(3)
  })

  it("ignores a stored value that is not a count", () => {
    localStorage.setItem("slashobserve-sidebar-rows", '{"logs":"many"}')
    expect(storedRows("logs")).toBe(0)
    localStorage.setItem("slashobserve-sidebar-rows", "not json")
    expect(storedRows("logs")).toBe(0)
  })

  it("draws one placeholder row for each reserved row and none for zero", () => {
    const { container, rerender } = render(
      <SubNavSkeleton label="Log sources" rows={3} />
    )
    expect(container.querySelectorAll("li")).toHaveLength(3)
    rerender(<SubNavSkeleton label="Log sources" rows={0} />)
    expect(container.querySelector("ul")).toBeNull()
  })
})
