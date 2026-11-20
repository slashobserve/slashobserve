import { screen, waitFor } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AddBlockRow } from "./add-block-row"
import type { Observation } from "./queries"

function renderRow(state = "open", canEdit = true) {
  return renderWithProviders(
    <AddBlockRow
      observation={{ id: "o1", state } as Observation}
      blocks={[]}
      canEdit={canEdit}
      picking={false}
      onPickingChange={() => {}}
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

describe("AddBlockRow on a resolved observation", () => {
  it("tells a commenter that a suggestion waits for an editor", async () => {
    renderRow("resolved", false)
    expect(
      await screen.findByText(
        "This observation is resolved. A block you suggest waits for an editor to keep it."
      )
    ).toBeTruthy()
  })

  it("tells an editor the page still takes blocks", async () => {
    renderRow("resolved", true)
    expect(
      await screen.findByText(/Blocks you add still go on the page/)
    ).toBeTruthy()
  })

  it("says nothing on an open observation", async () => {
    renderRow("open", false)
    await screen.findByRole("button", { name: "Text" })
    expect(screen.queryByText(/This observation is resolved/)).toBeNull()
  })
})

describe("AddBlockRow data pages", () => {
  it.each([
    ["Logs", "/logs"],
    ["Traces", "/traces"],
    ["Metrics", "/explore/metrics"],
  ])("opens %s with the observation as the way back", async (name, path) => {
    const { user, router } = renderRow()
    await user.click(await screen.findByRole("button", { name }))
    await waitFor(() => expect(router?.state.location.pathname).toBe(path))
    expect(router?.state.location.search).toEqual({ observation: "o1" })
  })
})
