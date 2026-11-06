import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import type { PageState } from "@/lib/paging"

import { PageBar } from "./page-bar"

const offsetState: PageState = {
  page: 1,
  pageSize: 50,
  rows: 50,
  total: 1284,
  totalRelation: "eq",
  hasNext: true,
  hasPrev: false,
  byOffset: true,
}

const cursorState: PageState = {
  page: 3,
  pageSize: 50,
  rows: 50,
  total: 200,
  totalRelation: "gte",
  hasNext: true,
  hasPrev: true,
  byOffset: false,
}

function bar(state: PageState, props: Partial<Parameters<typeof PageBar>[0]>) {
  return render(
    <PageBar
      state={state}
      noun="traces"
      qualifier=" in range"
      onPage={() => {}}
      onPageSize={() => {}}
      label="Trace pages"
      {...props}
    />
  )
}

describe("PageBar", () => {
  it("numbers the pages of an offset store up to the last one", () => {
    bar(offsetState, {})
    expect(screen.getByText("1-50 of 1,284 traces in range")).toBeVisible()
    const nav = screen.getByRole("navigation", { name: "Trace pages" })
    expect(nav).toBeVisible()
    expect(screen.getByRole("button", { name: "Page 26" })).toBeVisible()
    expect(screen.queryByRole("button", { name: "Page 27" })).toBeNull()
    expect(screen.getByRole("button", { name: "Page 1" })).toHaveAttribute(
      "aria-current",
      "page"
    )
  })

  it("shows the page number alone on a cursor store", () => {
    bar(cursorState, { note: "Tempo pages by start time." })
    expect(screen.getByText("Page 3")).toBeVisible()
    expect(screen.getByText("Tempo pages by start time.")).toBeVisible()
    expect(screen.queryByRole("button", { name: /^Page \d+$/ })).toBeNull()
  })

  it("shows only the current number when an offset store has no total", () => {
    bar(
      { ...offsetState, page: 2, total: undefined, totalRelation: "unknown" },
      {}
    )
    expect(screen.getByRole("button", { name: "Page 2" })).toHaveAttribute(
      "aria-current",
      "page"
    )
    expect(screen.queryByRole("button", { name: "Page 1" })).toBeNull()
  })

  it("disables Next on the last page and Previous on the first", async () => {
    const user = userEvent.setup()
    const onPage = vi.fn()
    bar({ ...offsetState, hasNext: false }, { onPage })
    const next = screen.getByRole("button", { name: "Next page" })
    const previous = screen.getByRole("button", { name: "Previous page" })
    expect(next).toHaveAttribute("aria-disabled", "true")
    expect(previous).toHaveAttribute("aria-disabled", "true")
    await user.click(next)
    await user.click(previous)
    expect(onPage).not.toHaveBeenCalled()
  })

  it("asks for the neighbouring and the numbered pages", async () => {
    const user = userEvent.setup()
    const onPage = vi.fn()
    bar({ ...offsetState, page: 2, hasPrev: true }, { onPage })
    await user.click(screen.getByRole("button", { name: "Next page" }))
    await user.click(screen.getByRole("button", { name: "Previous page" }))
    await user.click(screen.getByRole("button", { name: "Page 26" }))
    expect(onPage.mock.calls).toEqual([[3], [1], [26]])
  })

  it("marks the navigation busy while a page loads", () => {
    bar(offsetState, { busy: true })
    expect(
      screen.getByRole("navigation", { name: "Trace pages" })
    ).toHaveAttribute("aria-busy", "true")
  })

  it("offers the page sizes in a Select, not a native select", async () => {
    const user = userEvent.setup()
    const onPageSize = vi.fn()
    const { container } = bar(offsetState, { onPageSize })
    expect(container.querySelector("select")).toBeNull()
    await user.click(screen.getByRole("combobox", { name: "Rows per page" }))
    await user.click(
      await screen.findByRole("option", { name: "250 per page" })
    )
    expect(onPageSize).toHaveBeenCalledWith(250)
  })
})
