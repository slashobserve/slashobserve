import { act, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { ImportPage } from "./import-page"

vi.mock("./report-card", () => ({ ReportCard: () => null }))

const dashboardId = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"
const grafanaJson = JSON.stringify({ title: "Checkout", panels: [] })
const otherJson = JSON.stringify({ title: "Payments", panels: [] })

const preview = {
  summary: {
    format: "classic",
    title: "Checkout",
    uid: "checkout",
    schema_version: 39,
    bytes: 120,
    panels: 0,
    rows: 0,
    variables: 0,
  },
  datasources: [],
  existing: null,
  ready: true,
  report: {},
  spec: {},
}

const existing = {
  dashboard_id: dashboardId,
  title: "Checkout",
  version: 3,
  can_update: true,
}

const imported = {
  dashboard: {
    id: dashboardId,
    title: "Checkout",
    folder: { id: "f1", title: "General", general: true, personal: false },
  },
  version: { version: 1 },
  import: { datasources: [], report: {} },
  folder_note: null,
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function mockServer(options: {
  preview: unknown
  save: () => Response | Promise<Response>
}) {
  const previews: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url)
      if (pathname === "/api/v1/folders") return json({ folders: [] })
      if (pathname === "/api/v1/imports/preview") {
        previews.push(await request.clone().json())
        return json(options.preview)
      }
      if (pathname === "/api/v1/imports") return options.save()
      return json({ error: "not found", code: "not_found" }, 404)
    })
  )
  return { previews }
}

function heldReply() {
  let release = () => {}
  const reply = new Promise<Response>((resolve) => {
    release = () => resolve(json(imported, 201))
  })
  return { reply, release }
}

function renderPage() {
  return renderWithProviders(
    <ImportPage
      initial={{ name: "checkout.json", text: grafanaJson, pasted: false }}
    />,
    { route: { path: "/import", url: "/import" } }
  )
}

function pasteOnPage(text: string) {
  window.dispatchEvent(
    Object.assign(new Event("paste"), {
      clipboardData: { getData: () => text },
    })
  )
}

function dropOnPage(text: string) {
  window.dispatchEvent(
    Object.assign(new Event("drop"), {
      dataTransfer: {
        types: ["Files"],
        files: [new File([text], "payments.json")],
      },
    })
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("ImportPage", () => {
  it.each([
    ["pasted", pasteOnPage],
    ["dropped", dropOnPage],
  ])(
    "shows the import's result when other JSON is %s while it saves",
    async (_, send) => {
      const held = heldReply()
      const { previews } = mockServer({ preview, save: () => held.reply })
      const { user } = renderPage()
      await user.click(
        await screen.findByRole("button", { name: "Import as new dashboard" })
      )
      await screen.findByRole("button", { name: "Importing..." })

      await act(async () => {
        send(otherJson)
        await new Promise((resolve) => setTimeout(resolve, 0))
      })
      await act(async () => held.release())

      expect(
        await screen.findByText(/Imported as Checkout, version 1/)
      ).toBeInTheDocument()
      expect(screen.getByText("checkout.json")).toBeInTheDocument()
      expect(previews).toHaveLength(1)
    }
  )

  it("does not offer the page as a drop target while it saves", async () => {
    const held = heldReply()
    mockServer({ preview, save: () => held.reply })
    const { user } = renderPage()
    await screen.findByRole("button", { name: "Import as new dashboard" })
    const card = screen.getByRole("region", { name: "Dashboard JSON" })
    act(() => {
      window.dispatchEvent(
        Object.assign(new Event("dragenter"), {
          dataTransfer: { types: ["Files"] },
        })
      )
    })
    expect(card.dataset.dragging).toBe("true")

    await user.click(
      screen.getByRole("button", { name: "Import as new dashboard" })
    )
    await screen.findByRole("button", { name: "Importing..." })
    expect(card.dataset.dragging).toBeUndefined()

    await act(async () => held.release())
    await screen.findByText(/Imported as Checkout, version 1/)
    expect(card.dataset.dragging).toBe("true")
  })

  it("reads new JSON again once the import has finished", async () => {
    const { previews } = mockServer({
      preview,
      save: () => json(imported, 201),
    })
    const { user } = renderPage()
    await user.click(
      await screen.findByRole("button", { name: "Import as new dashboard" })
    )
    await screen.findByText(/Imported as Checkout, version 1/)

    act(() => pasteOnPage(otherJson))

    await waitFor(() => expect(previews).toHaveLength(2))
    expect(screen.getByText("Pasted JSON")).toBeInTheDocument()
    expect(screen.queryByText(/Imported as Checkout/)).toBeNull()
  })

  it("keeps the already imported notice beside an import error", async () => {
    mockServer({
      preview: { ...preview, existing },
      save: () => json({ error: "busy", code: "import_running" }, 429),
    })
    const { user } = renderPage()
    await user.click(
      await screen.findByRole("button", { name: "Update Checkout" })
    )

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Another import is still running. Try again in a moment."
    )
    expect(screen.getByText(/Already imported as/)).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Checkout" })).toBeInTheDocument()
  })
})
