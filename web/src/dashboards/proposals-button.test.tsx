import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ProposalsButton } from "./proposals-button"

const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"

function proposal(id: string, over: object = {}) {
  return {
    id,
    dashboard_id: dashboardId,
    state: "open",
    title: `Change ${id}`,
    revision: 1,
    base_version: 3,
    summary: "",
    author: { kind: "user", id: "u1", name: "Ada" },
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "mcp",
      client: "Claude Code",
      run_id: null,
      label: "Ada (via Claude Code)",
    },
    origin: "web",
    origin_client: null,
    run_id: null,
    created_at: "2026-11-11T10:00:00Z",
    updated_at: "2026-11-11T10:00:00Z",
    behind: false,
    latest_version: 3,
    allowed_actions: ["merge", "discard"],
    ...over,
  }
}

function serve(reply: Response | Promise<Response>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => reply)
  )
}

function list(proposals: object[], next_cursor: string | null = null) {
  return new Response(JSON.stringify({ proposals, next_cursor }), {
    headers: { "Content-Type": "application/json" },
  })
}

function renderButton() {
  return renderWithProviders(<ProposalsButton dashboardId={dashboardId} />, {
    route: { path: "/", url: "/" },
  })
}

afterEach(() => vi.unstubAllGlobals())

describe("ProposalsButton", () => {
  it("counts one open proposal in the singular", async () => {
    serve(list([proposal("p1")]))
    renderButton()
    expect(
      await screen.findByRole("button", { name: "1 proposal" })
    ).toBeTruthy()
  })

  it("lists each proposal with its author and whether it is behind", async () => {
    serve(list([proposal("p1"), proposal("p2", { behind: true })]))
    const { user } = renderButton()
    await user.click(await screen.findByRole("button", { name: "2 proposals" }))
    const links = await screen.findAllByRole("link")
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/proposals/p1",
      "/proposals/p2",
    ])
    expect(links[0]?.textContent).toContain("Ada (via Claude Code)")
    expect(links[0]?.textContent).not.toContain("Behind")
    expect(links[1]?.textContent).toContain("Behind")
  })

  it("shows a plus when more proposals are open than the page holds", async () => {
    serve(list([proposal("p1")], "next"))
    renderButton()
    expect(
      await screen.findByRole("button", { name: "1+ proposal" })
    ).toBeTruthy()
  })

  it("renders nothing with no open proposals", async () => {
    serve(list([]))
    const { container, queryClient } = renderButton()
    await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0))
    expect(container.querySelector("button")).toBeNull()
  })

  it("renders nothing while loading or when the list fails", async () => {
    serve(new Promise(() => {}))
    const loading = renderButton()
    expect(loading.container.querySelector("button")).toBeNull()
    loading.unmount()
    serve(
      new Response(JSON.stringify({ error: "x", code: "internal" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      })
    )
    const failed = renderButton()
    await vi.waitFor(() => expect(failed.queryClient.isFetching()).toBe(0))
    expect(failed.container.querySelector("button")).toBeNull()
  })
})
