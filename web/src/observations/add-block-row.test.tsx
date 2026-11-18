import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AddBlockRow } from "./add-block-row"
import type { Observation } from "./queries"

function renderRow() {
  return renderWithProviders(
    <AddBlockRow
      observation={{ id: "o1" } as Observation}
      blocks={[]}
      canEdit
      onAdded={() => {}}
    />,
    { route: { path: "/", url: "/" } }
  )
}

async function openText(user: ReturnType<typeof renderRow>["user"]) {
  await user.click(await screen.findByRole("button", { name: "Text" }))
  return screen.findByRole("textbox", { name: "New text block" })
}

describe("AddBlockRow text draft", () => {
  it("keeps what was typed when Escape closes the box", async () => {
    const { user } = renderRow()
    const box = await openText(user)
    await user.type(box, "Pool exhausted")
    await user.keyboard("{Escape}")
    expect(screen.queryByRole("textbox", { name: "New text block" })).toBeNull()
    const again = await openText(user)
    expect((again as HTMLTextAreaElement).value).toBe("Pool exhausted")
  })

  it("throws the draft away when Cancel is pressed", async () => {
    const { user } = renderRow()
    const box = await openText(user)
    await user.type(box, "Pool exhausted")
    await user.click(screen.getByRole("button", { name: "Cancel" }))
    const again = await openText(user)
    expect((again as HTMLTextAreaElement).value).toBe("")
  })

  it("says how to add the text without the mouse", async () => {
    const { user } = renderRow()
    const box = await openText(user)
    expect(box.getAttribute("placeholder")).toContain("Enter")
  })
})
