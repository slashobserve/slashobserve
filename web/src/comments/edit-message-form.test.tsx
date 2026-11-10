import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { EditMessageForm } from "./edit-message-form"

const message = { id: "message-1", body: "Latency looks high" }
const queryKey = ["comments"]

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function stubServer(respond: () => Response) {
  const sent: { url: string; method: string; body: unknown }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      sent.push({
        url: new URL(request.url).pathname,
        method: request.method,
        body: await request.json(),
      })
      return respond()
    })
  )
  return sent
}

function renderForm(onClose = vi.fn()) {
  const view = renderWithProviders(
    <EditMessageForm
      message={message}
      people={[]}
      queryKey={queryKey}
      onClose={onClose}
    />
  )
  return { ...view, onClose }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("EditMessageForm", () => {
  it("starts with the current body focused", () => {
    renderForm()
    expect(screen.getByRole("textbox", { name: "Edit comment" })).toHaveValue(
      "Latency looks high"
    )
    expect(screen.getByRole("textbox", { name: "Edit comment" })).toHaveFocus()
  })

  it("saves the trimmed new body to the message and closes", async () => {
    const sent = stubServer(() => json({ thread: {}, message: {} }))
    const { user, onClose } = renderForm()
    const field = screen.getByRole("textbox", { name: "Edit comment" })
    await user.clear(field)
    await user.type(field, "  Latency is fine now  ")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(sent).toEqual([
      {
        url: "/api/v1/messages/message-1",
        method: "PATCH",
        body: { body: "Latency is fine now" },
      },
    ])
  })

  it("closes without a request when the body did not change", async () => {
    const sent = stubServer(() => json({}))
    const { user, onClose } = renderForm()
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(onClose).toHaveBeenCalled()
    expect(sent).toEqual([])
  })

  it("does not save an empty body", async () => {
    const { user } = renderForm()
    await user.clear(screen.getByRole("textbox", { name: "Edit comment" }))
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled()
  })

  it("shows the server's reason and stays open when the save is refused", async () => {
    stubServer(() =>
      json({ error: "not your comment", code: "not_author" }, 403)
    )
    const { user, onClose } = renderForm()
    const field = screen.getByRole("textbox", { name: "Edit comment" })
    await user.type(field, " more")
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Not your comment"
    )
    expect(onClose).not.toHaveBeenCalled()
  })

  it("cancels with Escape and the Cancel button", async () => {
    const { user, onClose } = renderForm()
    await user.keyboard("{Escape}")
    expect(onClose).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole("button", { name: "Cancel" }))
    expect(onClose).toHaveBeenCalledTimes(2)
  })
})
