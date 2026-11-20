import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { ObservationPage } from "./observation-page"

vi.mock("./discussion-pane", () => ({
  DiscussionPane: ({ hidden }: { hidden: boolean }) => (
    <div data-testid="discussion" data-hidden={String(hidden)} />
  ),
}))

class QuietEventSource {
  static CLOSED = 2
  readyState = 0
  onerror: (() => void) | null = null
  addEventListener() {}
  close() {}
}

const author = {
  kind: "user",
  id: "u1",
  name: "Ada",
  origin: "web",
  client: null,
  run_id: null,
  label: "Ada",
}

const envelope = {
  observation: {
    id: "o1",
    folder_id: "f1",
    title: "Checkout slow",
    state: "open",
    summary: null,
    from: null,
    to: null,
    thread_id: null,
    run_id: null,
    url: "http://localhost/o/o1",
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    resolved_at: null,
    allowed_actions: ["update"],
    blocks: [],
  },
  folder: { id: "f1", title: "Incidents", personal: false },
  permissions: { edit: true, comment: true },
}

const replies: Record<string, unknown> = {
  "GET /api/v1/observations/o1": envelope,
  "GET /api/v1/observations/o1/rcas": { rcas: [] },
  "GET /api/v1/threads": { threads: [], messages: [], next_cursor: null },
  "GET /api/v1/dashboards": { dashboards: [], next_cursor: null },
}

beforeEach(() => {
  vi.stubGlobal("EventSource", QuietEventSource)
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      const body = replies[`${request.method} ${url.pathname}`]
      if (body === undefined) return new Response(null, { status: 599 })
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

function renderPage() {
  const host = document.createElement("div")
  document.body.append(host)
  return renderWithProviders(
    <PanelHostContext value={host}>
      <ObservationPage
        observationId="o1"
        rca={{ open: false, onOpenChange: () => {} }}
      />
    </PanelHostContext>,
    { route: { path: "/", url: "/" } }
  )
}

describe("ObservationPage right dock", () => {
  it("hides the discussion while the panel picker holds the right dock", async () => {
    const { user } = renderPage()
    const discussion = await screen.findByTestId("discussion")
    expect(discussion.dataset.hidden).toBe("false")

    await user.click(await screen.findByRole("button", { name: "Panel" }))

    await waitFor(() => expect(discussion.dataset.hidden).toBe("true"))
    await user.click(screen.getByRole("button", { name: "Close" }))
    await waitFor(() => expect(discussion.dataset.hidden).toBe("false"))
  })
})
