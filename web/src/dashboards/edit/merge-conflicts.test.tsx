import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import type { MergeConflict } from "@/proposals/queries"
import { renderWithProviders } from "@/test/render"

import { MergeConflicts } from "./merge-conflicts"

const titles = new Map([["latency", "Latency"]])

const conflicts: MergeConflict[] = [
  {
    kind: "setting",
    id: "time",
    reason: "both_changed",
    message: "Both changed the time range.",
  },
  {
    kind: "panel",
    id: "latency",
    reason: "both_changed",
    message: "Both changed the query.",
  },
  {
    kind: "variable",
    id: "env",
    reason: "changed_and_removed",
    message: "It was removed.",
  },
]

describe("MergeConflicts", () => {
  it("lists each overlapping unit by its panel title, variable or settings", () => {
    renderWithProviders(
      <MergeConflicts
        conflicts={conflicts}
        titles={titles}
        latest={5}
        onReload={() => {}}
      />
    )
    expect(screen.getByText("Version 5 changed the same things")).toBeTruthy()
    const items = screen
      .getAllByRole("listitem")
      .map((item) => item.textContent)
    expect(items).toEqual([
      "Latency: Both changed the query.",
      "Variable env: It was removed.",
      "Dashboard settings: Both changed the time range.",
    ])
  })

  it("reloads the latest version from the button", async () => {
    const onReload = vi.fn()
    const { user } = renderWithProviders(
      <MergeConflicts
        conflicts={conflicts}
        titles={titles}
        latest={5}
        onReload={onReload}
      />
    )
    await user.click(screen.getByRole("button", { name: "Reload version 5" }))
    expect(onReload).toHaveBeenCalledOnce()
  })

  it("says only that someone saved when the server named no units", () => {
    renderWithProviders(
      <MergeConflicts
        conflicts={[]}
        titles={titles}
        latest={5}
        onReload={() => {}}
      />
    )
    expect(
      screen.getByText("Someone saved version 5 since you started")
    ).toBeTruthy()
    expect(screen.queryByRole("list")).toBeNull()
    expect(
      screen.getByRole("button", { name: "Reload version 5" })
    ).toBeTruthy()
  })
})
