import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import type { DashboardSummary } from "./dashboard-columns"
import {
  ListSelectionProvider,
  RowSelectBox,
  TitleHeader,
  type ListSelection,
} from "./list-selection"

function dashboard(id: string, title: string) {
  return { id, title } as DashboardSummary
}

function selection(overrides: Partial<ListSelection> = {}): ListSelection {
  return {
    selectable: ["a", "b"],
    selected: new Set(),
    toggle: vi.fn(),
    togglePage: vi.fn(),
    ...overrides,
  }
}

describe("RowSelectBox", () => {
  it("names the dashboard and toggles it", async () => {
    const value = selection({ selected: new Set(["a"]) })
    render(
      <ListSelectionProvider value={value}>
        <RowSelectBox dashboard={dashboard("a", "Orders")} />
        <RowSelectBox dashboard={dashboard("b", "Fleet")} />
      </ListSelectionProvider>
    )
    expect(
      screen.getByRole("checkbox", { name: "Select Orders" })
    ).toBeChecked()
    const fleet = screen.getByRole("checkbox", { name: "Select Fleet" })
    expect(fleet).not.toBeChecked()
    await userEvent.click(fleet)
    expect(value.toggle).toHaveBeenCalledWith("b")
  })

  it("is left out for a dashboard the caller cannot move and with no selection", () => {
    const { rerender } = render(
      <ListSelectionProvider value={selection({ selectable: ["b"] })}>
        <RowSelectBox dashboard={dashboard("a", "Orders")} />
      </ListSelectionProvider>
    )
    expect(screen.queryByRole("checkbox")).toBeNull()
    rerender(
      <ListSelectionProvider value={null}>
        <RowSelectBox dashboard={dashboard("a", "Orders")} />
      </ListSelectionProvider>
    )
    expect(screen.queryByRole("checkbox")).toBeNull()
  })
})

describe("TitleHeader", () => {
  it("is plain text when nothing can be selected", () => {
    render(
      <ListSelectionProvider value={null}>
        <TitleHeader />
      </ListSelectionProvider>
    )
    expect(screen.getByText("Title")).toBeInTheDocument()
    expect(screen.queryByRole("checkbox")).toBeNull()
  })

  it("selects the whole page and shows a partial choice as mixed", async () => {
    const value = selection({ selected: new Set(["a"]) })
    render(
      <ListSelectionProvider value={value}>
        <TitleHeader />
      </ListSelectionProvider>
    )
    const box = screen.getByRole("checkbox", {
      name: "Select every dashboard on this page",
    })
    expect(box).toHaveAttribute("aria-checked", "mixed")
    await userEvent.click(box)
    expect(value.togglePage).toHaveBeenCalledOnce()
  })

  it("is checked when every selectable dashboard is chosen", () => {
    render(
      <ListSelectionProvider
        value={selection({ selected: new Set(["a", "b"]) })}
      >
        <TitleHeader />
      </ListSelectionProvider>
    )
    expect(
      screen.getByRole("checkbox", {
        name: "Select every dashboard on this page",
      })
    ).toBeChecked()
  })
})
