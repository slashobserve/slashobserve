import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { DeleteMessageButton } from "./delete-message-button"

afterEach(() => {
  vi.unstubAllGlobals()
})

function stub(status: number) {
  const sent: { url: string; method: string }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      sent.push({ url: new URL(request.url).pathname, method: request.method })
      return status === 204
        ? new Response(null, { status })
        : new Response(
            JSON.stringify({ error: "not your comment", code: "not_author" }),
            { status, headers: { "Content-Type": "application/json" } }
          )
    })
  )
  return sent
}

describe("DeleteMessageButton", () => {
  it("asks first and deletes the message once confirmed", async () => {
    const sent = stub(204)
    const user = userEvent.setup()
    renderWithProviders(
      <DeleteMessageButton
        messageId="m1"
        wholeThread={false}
        queryKey={["comments"]}
      />
    )
    await user.click(screen.getByRole("button", { name: "Delete" }))
    expect(await screen.findByText("Delete this comment?")).toBeVisible()
    expect(sent).toEqual([])
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete",
      })
    )
    await waitFor(() =>
      expect(sent).toEqual([{ url: "/api/v1/messages/m1", method: "DELETE" }])
    )
    await waitFor(() =>
      expect(screen.queryByText("Delete this comment?")).toBeNull()
    )
  })

  it("says the thread goes with it for the first comment", async () => {
    stub(204)
    const user = userEvent.setup()
    renderWithProviders(
      <DeleteMessageButton messageId="m1" wholeThread queryKey={["comments"]} />
    )
    await user.click(screen.getByRole("button", { name: "Delete" }))
    expect(await screen.findByText("Delete this thread?")).toBeVisible()
  })

  it("keeps the dialog open and shows the server's reason when it refuses", async () => {
    stub(403)
    const user = userEvent.setup()
    renderWithProviders(
      <DeleteMessageButton
        messageId="m1"
        wholeThread={false}
        queryKey={["comments"]}
      />
    )
    await user.click(screen.getByRole("button", { name: "Delete" }))
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete",
      })
    )
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Not your comment"
    )
  })
})
