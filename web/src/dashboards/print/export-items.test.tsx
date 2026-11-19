import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it } from "vitest"

import {
  DropdownMenu,
  DropdownMenuContent,
} from "@/components/ui/dropdown-menu"

import { ExportItems } from "./export-items"
import { useExportStore } from "./export-store"

afterEach(() => useExportStore.getState().finish())

function menu(blockedBy: string | null) {
  render(
    <DropdownMenu open>
      <DropdownMenuContent>
        <ExportItems blockedBy={blockedBy} />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

describe("ExportItems", () => {
  it("starts a print or an image from its item", async () => {
    menu(null)
    await userEvent.click(
      await screen.findByRole("menuitem", { name: "Print or save as PDF" })
    )
    expect(useExportStore.getState().mode).toBe("print")
    useExportStore.getState().finish()
    await userEvent.click(
      screen.getByRole("menuitem", { name: "Download as image" })
    )
    expect(useExportStore.getState().mode).toBe("image")
  })

  it("is off with the reason while the layout is being edited", async () => {
    menu("finish the layout first")
    for (const name of [
      "Print or save as PDF (finish the layout first)",
      "Download as image (finish the layout first)",
    ]) {
      expect(await screen.findByRole("menuitem", { name })).toHaveAttribute(
        "aria-disabled",
        "true"
      )
    }
  })
})
