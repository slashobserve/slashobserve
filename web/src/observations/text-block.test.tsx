import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { TextBlock } from "./text-block"
import type { Block } from "./queries"

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function textBlock(overrides: Partial<Block> = {}): Block {
  return {
    id: "b1",
    observation_id: "o1",
    kind: "text",
    state: "live",
    position: 0,
    width: "full",
    version: 3,
    body: { markdown: "Checkout latency doubled" },
    snapshot_id: null,
    run_id: null,
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    allowed_actions: ["update", "remove"],
    ...overrides,
  }
}

type Call = { method: string; path: string; body: unknown }

function stubServer(reply: { status: number; body: unknown }) {
  const calls: Call[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push({
        method: request.method,
        path: new URL(request.url).pathname,
        body: await request.clone().json(),
      })
      return new Response(JSON.stringify(reply.body), {
        status: reply.status,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return calls
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("TextBlock", () => {
  it("shows no Edit button without the update action", () => {
    renderWithProviders(
      <TextBlock block={textBlock({ allowed_actions: [] })} />
    )
    expect(screen.getByText("Checkout latency doubled")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull()
  })

  it("saves the typed text against the block's version with Cmd+Enter", async () => {
    const current = textBlock({
      version: 4,
      body: { markdown: "Checkout latency doubled after the deploy" },
    })
    const calls = stubServer({ status: 200, body: { block: current } })
    const { user } = renderWithProviders(<TextBlock block={textBlock()} />)
    await user.click(screen.getByRole("button", { name: "Edit" }))
    const field = screen.getByRole("textbox", { name: "Text block" })
    await user.clear(field)
    await user.type(field, "Checkout latency doubled after the deploy")
    await user.keyboard("{Meta>}{Enter}{/Meta}")
    await waitFor(() =>
      expect(calls).toEqual([
        {
          method: "PATCH",
          path: "/api/v1/blocks/b1",
          body: {
            version: 3,
            body: { markdown: "Checkout latency doubled after the deploy" },
          },
        },
      ])
    )
    await waitFor(() =>
      expect(screen.queryByRole("textbox", { name: "Text block" })).toBeNull()
    )
    expect(screen.getByRole("button", { name: "Edit" })).toHaveFocus()
  })

  it("keeps the typed text on a version conflict and offers their version", async () => {
    const theirs = textBlock({
      version: 5,
      body: { markdown: "Someone else wrote this" },
    })
    const calls = stubServer({
      status: 409,
      body: {
        error: "The block changed.",
        code: "version_conflict",
        current: theirs,
      },
    })
    const { user } = renderWithProviders(<TextBlock block={textBlock()} />)
    await user.click(screen.getByRole("button", { name: "Edit" }))
    const field = screen.getByRole("textbox", { name: "Text block" })
    await user.clear(field)
    await user.type(field, "My own words")
    await user.click(screen.getByRole("button", { name: "Save" }))

    expect(
      await screen.findByText(
        "Someone changed this block while you were editing"
      )
    ).toBeInTheDocument()
    expect(field).toHaveValue("My own words")

    await user.click(screen.getByRole("button", { name: "Use their version" }))
    expect(field).toHaveValue("Someone else wrote this")
    expect(
      screen.queryByRole("button", { name: "Use their version" })
    ).toBeNull()
    expect(calls).toHaveLength(1)
  })

  it("does not save an empty text and says why", async () => {
    const calls = stubServer({ status: 200, body: {} })
    const { user } = renderWithProviders(<TextBlock block={textBlock()} />)
    await user.click(screen.getByRole("button", { name: "Edit" }))
    await user.clear(screen.getByRole("textbox", { name: "Text block" }))
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(
      await screen.findByText("Write something, or delete the block")
    ).toBeInTheDocument()
    expect(calls).toHaveLength(0)
  })

  it("cancels with Escape and returns focus to Edit", async () => {
    const { user } = renderWithProviders(<TextBlock block={textBlock()} />)
    await user.click(screen.getByRole("button", { name: "Edit" }))
    await user.keyboard("{Escape}")
    expect(screen.queryByRole("textbox", { name: "Text block" })).toBeNull()
    expect(screen.getByRole("button", { name: "Edit" })).toHaveFocus()
  })
})
