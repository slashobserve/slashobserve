import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { FilterToolbar } from "./filter-toolbar"

describe("FilterToolbar", () => {
  it("names the group and renders the filters and the summary", () => {
    render(
      <FilterToolbar label="Filter activity" summary="12 of 40 events">
        <input aria-label="Search" />
      </FilterToolbar>
    )
    const group = screen.getByRole("group", { name: "Filter activity" })
    expect(within(group).getByRole("textbox", { name: "Search" })).toBeVisible()
    expect(within(group).getByText("12 of 40 events")).toBeInTheDocument()
  })

  it("calls onClear from Clear filters", async () => {
    const user = userEvent.setup()
    const onClear = vi.fn()
    render(
      <FilterToolbar label="Filter activity" onClear={onClear}>
        <input aria-label="Search" />
      </FilterToolbar>
    )
    await user.click(screen.getByRole("button", { name: "Clear filters" }))
    expect(onClear).toHaveBeenCalledOnce()
  })

  it("has no Clear filters without onClear", () => {
    render(
      <FilterToolbar label="Filter activity">
        <input aria-label="Search" />
      </FilterToolbar>
    )
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull()
  })
})
