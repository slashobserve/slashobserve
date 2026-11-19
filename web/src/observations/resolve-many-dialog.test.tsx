import { screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ResolveManyDialog } from "./resolve-many-dialog"

function renderDialog(count: number) {
  const targets = Array.from({ length: count }, (_, i) => ({
    id: `o-${i}`,
    title: `Observation ${i}`,
  }))
  return renderWithProviders(
    <ResolveManyDialog
      targets={targets}
      open
      onOpenChange={vi.fn()}
      onResolved={vi.fn()}
    />,
    { route: { path: "/", url: "/" } }
  )
}

describe("ResolveManyDialog", () => {
  it("says the one summary applies to every selected observation", async () => {
    renderDialog(3)
    expect(
      await screen.findByText(
        "This summary applies to all 3 and shows under each title and in the list."
      )
    ).toBeTruthy()
    expect(screen.getByRole("button", { name: "Resolve 3" })).toBeTruthy()
  })

  it("keeps the single wording for one observation", async () => {
    renderDialog(1)
    expect(
      await screen.findByText(
        "The summary shows under the title and in the list."
      )
    ).toBeTruthy()
  })
})
