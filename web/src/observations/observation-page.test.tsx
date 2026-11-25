import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { ObservationPage } from "./observation-page"

vi.mock("@/query-editor/code-editor", () => ({
  CodeEditor: ({ value, ariaLabel }: { value: string; ariaLabel: string }) => (
    <textarea aria-label={ariaLabel} value={value} readOnly />
  ),
}))

vi.mock("./discussion-pane", () => ({
  DiscussionPane: ({
    hidden,
    focusedThreadId,
  }: {
    hidden: boolean
    focusedThreadId?: string
  }) => (
    <div
      data-testid="discussion"
      data-hidden={String(hidden)}
      data-focused-thread={focusedThreadId}
    />
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

const panelBlock = {
  id: "b1",
  observation_id: "o1",
  kind: "panel",
  state: "live",
  position: 0,
  width: "full",
  version: 1,
  body: {
    panel: {
      type: "timeseries",
      title: "Errors",
      datasource: "prom",
      queries: [{ query: "up" }],
    },
  },
  allowed_actions: ["update", "remove"],
  created_by: author,
  created_at: "2026-11-12T10:00:00Z",
  updated_at: "2026-11-12T10:00:00Z",
}

let replies: Record<string, unknown> = {}

const baseReplies: Record<string, unknown> = {
  "GET /api/v1/observations/o1": envelope,
  "GET /api/v1/observations/o1/rcas": { rcas: [] },
  "GET /api/v1/threads": { threads: [], messages: [], next_cursor: null },
  "GET /api/v1/dashboards": { dashboards: [], next_cursor: null },
  "GET /api/v1/datasources": {
    datasources: [
      { uid: "prom", name: "Prometheus", type: "prometheus", health: "ok" },
    ],
  },
}

beforeEach(() => {
  replies = { ...baseReplies }
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

function renderPage(focusedThreadId?: string) {
  const host = document.createElement("div")
  document.body.append(host)
  return renderWithProviders(
    <PanelHostContext value={host}>
      <ObservationPage
        observationId="o1"
        rca={{ open: false, onOpenChange: () => {} }}
        focusedThreadId={focusedThreadId}
      />
    </PanelHostContext>,
    { route: { path: "/", url: "/" } }
  )
}

describe("ObservationPage focused thread", { timeout: 20_000 }, () => {
  const originalWidth = window.innerWidth

  afterEach(() => {
    window.innerWidth = originalWidth
  })

  it("opens the discussion on a narrow screen and passes the thread to it", async () => {
    window.innerWidth = 500
    renderPage("t1")
    const discussion = await screen.findByTestId("discussion")
    expect(discussion.dataset.focusedThread).toBe("t1")
  })

  it("leaves the discussion closed on a narrow screen with no thread", async () => {
    window.innerWidth = 500
    renderPage()
    await screen.findByRole("heading", { name: "Checkout slow" })
    expect(screen.queryByTestId("discussion")).toBeNull()
  })
})

describe("ObservationPage right dock", { timeout: 20_000 }, () => {
  it("hides the discussion while the panel picker holds the right dock", async () => {
    const { user } = renderPage()
    const discussion = await screen.findByTestId("discussion")
    expect(discussion.dataset.hidden).toBe("false")

    await user.click(await screen.findByRole("button", { name: "Panel" }))
    await user.click(
      await screen.findByRole("menuitem", { name: "From a dashboard" })
    )

    await waitFor(() => expect(discussion.dataset.hidden).toBe("true"))
    await user.click(screen.getByRole("button", { name: "Close" }))
    await waitFor(() => expect(discussion.dataset.hidden).toBe("false"))
  })

  it("opens the panel editor on a new query and gives the dock back on close", async () => {
    const { user } = renderPage()
    const discussion = await screen.findByTestId("discussion")

    await user.click(await screen.findByRole("button", { name: "Panel" }))
    await user.click(await screen.findByRole("menuitem", { name: "New query" }))

    expect(
      await screen.findByRole("heading", { name: "New query" })
    ).toBeTruthy()
    expect(discussion.dataset.hidden).toBe("true")
    await user.click(screen.getByRole("button", { name: "Close panel editor" }))
    await waitFor(() => expect(discussion.dataset.hidden).toBe("false"))
  })

  it("opens the panel editor on a panel block from its menu", async () => {
    replies["GET /api/v1/observations/o1"] = {
      ...envelope,
      observation: { ...envelope.observation, blocks: [panelBlock] },
    }
    const { user } = renderPage()

    await user.click(
      await screen.findByRole("button", { name: "Panel menu: Errors" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Edit panel" })
    )

    expect(
      await screen.findByRole("heading", { name: "Edit Errors" })
    ).toBeTruthy()
  })

  it("asks before another opener replaces a changed draft", async () => {
    replies["GET /api/v1/observations/o1"] = {
      ...envelope,
      observation: { ...envelope.observation, blocks: [panelBlock] },
    }
    const { user } = renderPage()
    await user.click(
      await screen.findByRole("button", { name: "Panel menu: Errors" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: "Edit panel" })
    )
    const title = await screen.findByLabelText("Title", {}, { timeout: 5000 })
    await user.clear(title)
    await user.type(title, "Checkout errors")

    for (const item of ["New query", "From a dashboard"]) {
      await user.click(screen.getByRole("button", { name: "Panel" }))
      await user.click(await screen.findByRole("menuitem", { name: item }))
      expect(
        await screen.findByRole("alertdialog", {
          name: "Discard your changes?",
        })
      ).toBeTruthy()
      await user.click(screen.getByRole("button", { name: "Keep editing" }))
      await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
      expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe(
        "Checkout errors"
      )
    }

    await user.click(screen.getByRole("button", { name: "Panel" }))
    await user.click(await screen.findByRole("menuitem", { name: "New query" }))
    await user.click(await screen.findByRole("button", { name: "Discard" }))
    expect(
      await screen.findByRole("heading", { name: "New query" })
    ).toBeTruthy()
  })
})
