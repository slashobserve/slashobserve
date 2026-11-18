import { fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AnnotationActionsProvider } from "./actions"
import { AnnotationControls } from "./annotation-controls"
import { AnnotationLayer } from "./annotation-layer"
import type { Annotation } from "./queries"

vi.mock("sonner", () => ({ toast: { success: vi.fn() } }))

const t0 = Date.parse("2026-11-01T10:00:00Z")
const minute = 60_000

const mine: Annotation = {
  id: "a1",
  dashboard_id: "d1",
  panel_id: null,
  time_from: new Date(t0 + 10 * minute).toISOString(),
  time_to: null,
  text: "Deploy checkout",
  tags: ["deploy"],
  created_by: "u1",
  created_by_name: "Ada",
  actor: null,
  created_at: new Date(t0).toISOString(),
}

type Call = { method: string; url: string; body: unknown }

function stubApi(
  respond: (call: Call) => { status: number; body: unknown } = () => ({
    status: 200,
    body: mine,
  })
) {
  const calls: Call[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const text = await request.clone().text()
      const call = {
        method: request.method,
        url: new URL(request.url).pathname,
        body: text === "" ? undefined : JSON.parse(text),
      }
      calls.push(call)
      const { status, body } = respond(call)
      return new Response(status === 204 ? null : JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return calls
}

function page(over: { canEdit?: boolean } = {}) {
  return renderWithProviders(
    <AnnotationActionsProvider
      dashboardId="d1"
      zone="UTC"
      panelTitle={(id) => id}
      canEdit={over.canEdit ?? true}
      principal={{ id: "u1", role: "editor" }}
    >
      <AnnotationControls />
      <AnnotationLayer
        annotations={[mine]}
        fromMs={t0}
        toMs={t0 + 100 * minute}
        left={0}
        top={0}
        width={1000}
        height={200}
        zone="UTC"
      />
    </AnnotationActionsProvider>,
    { route: { path: "/", url: "/" } }
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("annotation actions", () => {
  it("offers Add annotation only to someone who can edit the dashboard", async () => {
    page({ canEdit: false })
    await screen.findByRole("button", { name: /^Annotation:/ })
    expect(screen.queryByRole("button", { name: "Add annotation" })).toBeNull()
  })

  it("refuses an empty text without calling the server", async () => {
    const calls = stubApi()
    const { user } = page()
    await user.click(
      await screen.findByRole("button", { name: "Add annotation" })
    )
    await user.click(await screen.findByRole("button", { name: "Add" }))
    expect(await screen.findByText("Enter what happened.")).toBeVisible()
    expect(calls).toEqual([])
  })

  it("rejects a time outside the supported years", async () => {
    const calls = stubApi()
    const { user } = page()
    await user.click(
      await screen.findByRole("button", { name: "Add annotation" })
    )
    await user.type(await screen.findByLabelText("What happened"), "Restart")
    const from = screen.getByLabelText("From")
    await user.clear(from)
    await user.type(from, "9999-01-01 00:00")
    await user.click(screen.getByRole("button", { name: "Add" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(/2099/)
    expect(calls).toEqual([])
  })

  it("posts the new annotation for this dashboard", async () => {
    const calls = stubApi(() => ({ status: 201, body: mine }))
    const { user } = page()
    await user.click(
      await screen.findByRole("button", { name: "Add annotation" })
    )
    await user.type(await screen.findByLabelText("What happened"), "Restart")
    await user.type(screen.getByLabelText("Tags"), "ops, restart")
    await user.click(screen.getByRole("button", { name: "Add" }))
    await waitFor(() =>
      expect(calls.some((c) => c.method === "POST")).toBe(true)
    )
    const post = calls.find((c) => c.method === "POST")!
    expect(post.url).toBe("/api/v1/annotations")
    expect(post.body).toMatchObject({
      dashboard_id: "d1",
      text: "Restart",
      tags: ["ops", "restart"],
    })
  })

  it("shows the server's refusal and keeps the form open", async () => {
    stubApi(() => ({
      status: 422,
      body: {
        code: "unknown_panel",
        error: "that panel is not on this dashboard",
      },
    }))
    const { user } = page()
    await user.click(
      await screen.findByRole("button", { name: "Add annotation" })
    )
    await user.type(await screen.findByLabelText("What happened"), "Restart")
    await user.click(screen.getByRole("button", { name: "Add" }))
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "That panel is not on this dashboard."
    )
    expect(screen.getByRole("button", { name: "Try again" })).toBeVisible()
  })

  it("patches the text and tags of an annotation from its card", async () => {
    const calls = stubApi()
    const { user } = page()
    await user.hover(
      await screen.findByRole("button", { name: /^Annotation:/ })
    )
    fireEvent.click(screen.getByRole("button", { name: "Edit" }))
    const text = await screen.findByLabelText("What happened")
    await user.clear(text)
    await user.type(text, "Deploy checkout v2")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() =>
      expect(calls.some((c) => c.method === "PATCH")).toBe(true)
    )
    const patch = calls.find((c) => c.method === "PATCH")!
    expect(patch.url).toBe("/api/v1/annotations/a1")
    expect(patch.body).toEqual({ text: "Deploy checkout v2", tags: ["deploy"] })
  })

  it("deletes an annotation after confirmation", async () => {
    const calls = stubApi(() => ({ status: 204, body: null }))
    const { user } = page()
    await user.hover(
      await screen.findByRole("button", { name: /^Annotation:/ })
    )
    fireEvent.click(screen.getByRole("button", { name: "Delete" }))
    expect(await screen.findByText("Delete this annotation?")).toBeVisible()
    expect(calls).toEqual([])
    await user.click(screen.getByRole("button", { name: "Delete" }))
    await waitFor(() =>
      expect(calls.some((c) => c.method === "DELETE")).toBe(true)
    )
    expect(calls.find((c) => c.method === "DELETE")!.url).toBe(
      "/api/v1/annotations/a1"
    )
  })
})
