import { screen, waitFor } from "@testing-library/react"
import { createRef } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"
import { PanelHostContext } from "@/shell/side-panel"

import { DatasourcePanel } from "./datasource-panel"

const config = {
  uid: "payroll",
  type: "prometheus",
  name: "payroll",
  audience: "everyone",
  source: "app",
  address: "https://prom.example.com",
  secrets_set: [],
  created_at: "2026-11-19T10:00:00Z",
  updated_at: "2026-11-19T10:00:00Z",
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

function renderEditor(
  patch: (body: unknown) => Response,
  saved: typeof config = config
) {
  const sent: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname
      if (path === "/api/v1/datasources/payroll/config") return json(saved)
      if (path === "/api/v1/datasources/test") return json({ healthy: true })
      if (
        path === "/api/v1/datasources/payroll" &&
        request.method === "PATCH"
      ) {
        const body: unknown = await request.json()
        sent.push(body)
        return patch(body)
      }
      return new Response("{}", { status: 404 })
    })
  )
  const host = document.createElement("div")
  document.body.append(host)
  const view = renderWithProviders(
    <PanelHostContext.Provider value={host}>
      <DatasourcePanel
        target={{ mode: "edit", uid: "payroll" }}
        onClose={() => undefined}
        trigger={createRef()}
        onPickMcp={() => undefined}
      />
    </PanelHostContext.Provider>,
    { route: { path: "/", url: "/" } }
  )
  return { ...view, sent }
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("DatasourcePanel title", () => {
  it("names the datasource by its name, not its id", async () => {
    renderEditor(() => json(config), { ...config, name: "Payroll database" })
    expect(
      await screen.findByRole("heading", { name: "Edit Payroll database" })
    ).toBeInTheDocument()
  })

  it("falls back to the id when the datasource has no other name", async () => {
    renderEditor(() => json(config))
    expect(
      await screen.findByRole("heading", { name: "Edit payroll" })
    ).toBeInTheDocument()
  })
})

describe("DatasourcePanel audience", () => {
  it("starts on the saved audience and says what Admins only means", async () => {
    renderEditor(() => json(config))
    expect(await screen.findByRole("radio", { name: "Everyone" })).toBeChecked()
    expect(screen.getByRole("radio", { name: "Admins only" })).not.toBeChecked()
    expect(
      screen.getByText(/must live in a folder set to Only Admins/)
    ).toBeInTheDocument()
  })

  it("sends the chosen audience with the other edits", async () => {
    const { user, sent } = renderEditor(() =>
      json({ ...config, audience: "admins" })
    )
    await user.click(await screen.findByRole("radio", { name: "Admins only" }))
    await user.type(screen.getByLabelText("Name"), "Payroll")
    await user.click(screen.getByRole("button", { name: "Save changes" }))
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]).toMatchObject({ audience: "admins", name: "Payroll" })
  })

  it("lists what blocks Only Admins and keeps the person's edits", async () => {
    const { user } = renderEditor(() =>
      json(
        {
          error: "move them first",
          code: "restricted_content",
          datasource: "payroll",
          items: [
            {
              kind: "dashboard",
              id: "11111111-1111-1111-1111-111111111111",
              title: "Salaries",
              folder_id: "f1",
              folder_title: "Finance",
            },
            {
              kind: "saved_search",
              id: "22222222-2222-2222-2222-222222222222",
              title: "Pay errors",
              folder_id: "f2",
              folder_title: "Ops",
            },
          ],
          unlisted: 4,
          hidden: 3,
        },
        409
      )
    )
    await user.click(await screen.findByRole("radio", { name: "Admins only" }))
    await user.type(screen.getByLabelText("Name"), "Payroll")
    await user.click(screen.getByRole("button", { name: "Save changes" }))
    expect(
      await screen.findByRole("link", { name: "Salaries" })
    ).toHaveAttribute("href", expect.stringContaining("/d/1111"))
    expect(screen.getByRole("link", { name: "Pay errors" })).toHaveAttribute(
      "href",
      "/f/f2"
    )
    expect(screen.getByText(/Dashboard:/)).toHaveTextContent("in Finance")
    expect(screen.getByText("and 4 more")).toBeInTheDocument()
    expect(screen.getByText("3 in personal folders")).toBeInTheDocument()
    expect(
      screen.getByText(/Move them to an Only Admins folder/)
    ).toBeInTheDocument()
    expect(screen.getByLabelText("Name")).toHaveValue("Payroll")
    expect(screen.getByRole("radio", { name: "Admins only" })).toBeChecked()

    await user.click(screen.getByRole("radio", { name: "Everyone" }))
    expect(
      screen.queryByRole("link", { name: "Salaries" })
    ).not.toBeInTheDocument()
  })
})

describe("DatasourcePanel test result", () => {
  it("scrolls the result into view so it is not left below the fold", async () => {
    const reveal = vi.fn()
    Element.prototype.scrollIntoView = reveal
    const { user } = renderEditor(() => json(config))
    await user.click(
      await screen.findByRole("button", { name: "Test connection" })
    )
    expect(await screen.findByText(/Connected\./)).toBeInTheDocument()
    expect(reveal).toHaveBeenCalledWith({ block: "nearest" })
  })
})
