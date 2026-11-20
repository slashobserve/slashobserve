import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { SortMenu, sortText } from "./sort-menu"

describe("sortText", () => {
  it("names each order the list can be sorted by", () => {
    expect(sortText(undefined)).toBe("by folder")
    expect(sortText("updated")).toBe("last updated")
    expect(sortText("name")).toBe("name")
    expect(sortText("views")).toBe("most viewed")
  })
})

describe("SortMenu", () => {
  it("offers folder, last updated, name and most viewed and reports the choice", async () => {
    const onChange = vi.fn()
    renderWithProviders(<SortMenu value={undefined} onChange={onChange} />)
    await userEvent.click(
      screen.getByRole("button", { name: /Sort: by folder/ })
    )
    const labels = screen
      .getAllByRole("menuitemradio")
      .map((item) => item.textContent)
    expect(labels).toEqual(["By folder", "Last updated", "Name", "Most viewed"])
    await userEvent.click(screen.getByRole("menuitemradio", { name: "Name" }))
    expect(onChange).toHaveBeenCalledWith("name")
  })

  it("goes back to the folder grouping with no sort", async () => {
    const onChange = vi.fn()
    renderWithProviders(<SortMenu value="name" onChange={onChange} />)
    await userEvent.click(screen.getByRole("button", { name: /Sort: name/ }))
    await userEvent.click(
      screen.getByRole("menuitemradio", { name: "By folder" })
    )
    expect(onChange).toHaveBeenCalledWith(undefined)
  })
})
