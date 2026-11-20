import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { RequestCommentAccess } from "./request-comment-access"

const dashboardId = "3f2c8d1e-0b5a-4c7d-9e11-2a6b8c4d5e70"

afterEach(() => vi.unstubAllGlobals())

function answer(status: number, body: unknown) {
  const bodies: unknown[] = []
  const fetchMock = vi.fn(async (input: Request) => {
    bodies.push(await input.clone().json())
    return new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    })
  })
  vi.stubGlobal("fetch", fetchMock)
  return { fetchMock, bodies }
}

const created = {
  id: "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71",
  object: { kind: "dashboard", id: dashboardId, title: "Checkout" },
  requested_by: { kind: "user", id: "u1", name: "Ben" },
  note: "",
  created_at: "2026-11-20T10:00:00Z",
}

function renderButton() {
  return renderWithProviders(
    <RequestCommentAccess object={{ kind: "dashboard", id: dashboardId }} />
  )
}

describe("RequestCommentAccess", () => {
  it("asks for a note before it sends anything", async () => {
    const { fetchMock } = answer(201, created)
    const { user } = renderButton()
    await user.click(
      screen.getByRole("button", { name: "Request comment access" })
    )
    expect(
      screen.getByRole("textbox", {
        name: "Note for the people who can grant access (optional)",
      })
    ).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("sends the object and the trimmed note, then says it was sent", async () => {
    const { bodies } = answer(201, created)
    const { user } = renderButton()
    await user.click(
      screen.getByRole("button", { name: "Request comment access" })
    )
    await user.type(screen.getByRole("textbox"), "  on call this week  ")
    await user.click(screen.getByRole("button", { name: "Send request" }))
    expect(await screen.findByRole("status")).toHaveTextContent("Request sent.")
    expect(bodies).toEqual([
      {
        object: { kind: "dashboard", id: dashboardId },
        note: "on call this week",
      },
    ])
    expect(
      screen.queryByRole("button", { name: "Request comment access" })
    ).toBeNull()
  })

  it("leaves the note out when it is empty", async () => {
    const { bodies } = answer(201, created)
    const { user } = renderButton()
    await user.click(
      screen.getByRole("button", { name: "Request comment access" })
    )
    await user.click(screen.getByRole("button", { name: "Send request" }))
    await screen.findByRole("status")
    expect(bodies).toEqual([{ object: { kind: "dashboard", id: dashboardId } }])
  })

  it.each([
    [
      "request_open",
      "You already asked for comment access here, and the request is still open.",
    ],
    [
      "already_can_comment",
      "You can already comment here. Reload the page to start.",
    ],
  ])("answers %s with what is true now", async (code, text) => {
    answer(409, { error: "refused", code })
    const { user } = renderButton()
    await user.click(
      screen.getByRole("button", { name: "Request comment access" })
    )
    await user.click(screen.getByRole("button", { name: "Send request" }))
    expect(await screen.findByRole("status")).toHaveTextContent(text)
  })

  it("keeps the form after a failure so the person can try again", async () => {
    const { fetchMock } = answer(500, {
      error: "internal error",
      code: "internal",
    })
    const { user } = renderButton()
    await user.click(
      screen.getByRole("button", { name: "Request comment access" })
    )
    await user.click(screen.getByRole("button", { name: "Send request" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The request was not sent."
    )
    await user.click(screen.getByRole("button", { name: "Send request" }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  })

  it("goes back to the button on Cancel", async () => {
    answer(201, created)
    const { user } = renderButton()
    await user.click(
      screen.getByRole("button", { name: "Request comment access" })
    )
    await user.click(screen.getByRole("button", { name: "Cancel" }))
    expect(
      screen.getByRole("button", { name: "Request comment access" })
    ).toBeInTheDocument()
  })
})
