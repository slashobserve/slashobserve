import { createRef } from "react"
import { screen, waitFor } from "@testing-library/react"
import { toast } from "sonner"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { DataPanel } from "@/spec"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import { AddToObservationPanel } from "./add-to-observation-panel"

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function observation(id: string, title: string) {
  return {
    id,
    folder_id: "f1",
    title,
    state: "open",
    summary: null,
    from: null,
    to: null,
    thread_id: null,
    run_id: null,
    url: `http://localhost/o/${id}`,
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    resolved_at: null,
    allowed_actions: [],
  }
}

function blockReply(state: "live" | "requested") {
  return {
    block: {
      id: "b1",
      observation_id: "o1",
      kind: "panel",
      state,
      position: 0,
      width: "full",
      version: 1,
      body: {},
      snapshot_id: null,
      run_id: null,
      created_by: author,
      created_at: "2026-11-12T10:00:00Z",
      updated_at: "2026-11-12T10:00:00Z",
      allowed_actions: [],
    },
  }
}

const panel = {
  id: "latency",
  type: "timeseries",
  title: "Latency",
  datasource: "prom",
  gridPos: { x: 4, y: 9, w: 12, h: 6 },
  queries: [{ query: "rate(http_requests_total[5m])" }],
} as DataPanel

type Reply = { status: number; body: unknown }
type Call = { method: string; path: string; body: unknown }

function stubServer(replies: Record<string, Reply>) {
  const calls: Call[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      const text = await request.clone().text()
      calls.push({
        method: request.method,
        path: url.pathname,
        body: text === "" ? undefined : JSON.parse(text),
      })
      const reply = replies[`${request.method} ${url.pathname}`]
      if (reply === undefined) return new Response(null, { status: 599 })
      return new Response(JSON.stringify(reply.body), {
        status: reply.status,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return calls
}

function openList(...items: ReturnType<typeof observation>[]): Reply {
  return { status: 200, body: { observations: items, next_cursor: null } }
}

function render(
  onClose = vi.fn(),
  defaultObservationId?: string,
  range = { from: "now-6h", to: "now" }
) {
  const host = document.createElement("div")
  document.body.append(host)
  const trigger = createRef<HTMLElement>()
  const rendered = renderWithProviders(
    <PanelHostContext value={host}>
      <AddToObservationPanel
        panel={panel}
        from={range.from}
        to={range.to}
        trigger={trigger}
        onClose={onClose}
        defaultObservationId={defaultObservationId}
      />
    </PanelHostContext>,
    { route: { path: "/", url: "/" } }
  )
  return { ...rendered, onClose }
}

const foldersReply: Reply = { status: 200, body: { folders: [] } }

beforeEach(() => {
  vi.mocked(toast.success).mockClear()
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

describe("AddToObservationPanel", () => {
  it("adds the panel without its id and layout to the chosen open observation", async () => {
    const calls = stubServer({
      "GET /api/v1/observations": openList(
        observation("o1", "Checkout slow"),
        observation("o2", "Payments down")
      ),
      "GET /api/v1/folders": foldersReply,
      "POST /api/v1/observations/o2/blocks": {
        status: 201,
        body: blockReply("live"),
      },
    })
    const { user, onClose } = render()
    await user.click(await screen.findByRole("combobox"))
    await user.click(
      await screen.findByRole("option", { name: "Payments down" })
    )
    await user.click(screen.getByRole("button", { name: "Add" }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    const post = calls.find((call) => call.method === "POST")
    expect(post?.path).toBe("/api/v1/observations/o2/blocks")
    const body = post?.body as {
      kind: string
      body: { panel: Record<string, unknown>; from: string; to: string }
    }
    expect(body.kind).toBe("panel")
    expect(body.body.panel).toEqual({
      type: "timeseries",
      title: "Latency",
      datasource: "prom",
      queries: [{ query: "rate(http_requests_total[5m])" }],
    })
    expect(Date.parse(body.body.to) - Date.parse(body.body.from)).toBe(
      6 * 3_600_000
    )
    expect(toast.success).toHaveBeenCalledWith(
      "Added to Payments down",
      expect.objectContaining({
        action: expect.objectContaining({ label: "Open" }),
      })
    )
  })

  it.each([
    ["on the first page", true],
    ["beyond the first page", false],
  ])("preselects the default observation %s", async (_, listed) => {
    const wanted = observation("o5", "Disk full")
    const calls = stubServer({
      "GET /api/v1/observations": openList(
        ...(listed ? [wanted] : [observation("o1", "Checkout slow")])
      ),
      "GET /api/v1/observations/o5": {
        status: 200,
        body: { observation: wanted, folder: {}, permissions: {} },
      },
      "GET /api/v1/folders": foldersReply,
      "POST /api/v1/observations/o5/blocks": {
        status: 201,
        body: blockReply("live"),
      },
    })
    const { user, onClose } = render(vi.fn(), "o5")
    await waitFor(() =>
      expect(screen.getByRole("combobox")).toHaveValue("Disk full")
    )
    await user.click(screen.getByRole("button", { name: "Add" }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(
      calls.some((call) => call.path === "/api/v1/observations/o5/blocks")
    ).toBe(true)
  })

  it("says suggested when the block comes back requested", async () => {
    stubServer({
      "GET /api/v1/observations": openList(observation("o1", "Checkout slow")),
      "GET /api/v1/folders": foldersReply,
      "POST /api/v1/observations/o1/blocks": {
        status: 201,
        body: blockReply("requested"),
      },
    })
    const { user, onClose } = render()
    await user.click(await screen.findByRole("combobox"))
    await user.click(
      await screen.findByRole("option", { name: "Checkout slow" })
    )
    await user.click(screen.getByRole("button", { name: "Add" }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(toast.success).toHaveBeenCalledWith(
      "Suggested on Checkout slow",
      expect.anything()
    )
  })

  it("switches to a new observation when none is open, then creates it and adds the block", async () => {
    const calls = stubServer({
      "GET /api/v1/observations": openList(),
      "GET /api/v1/folders": foldersReply,
      "POST /api/v1/observations": {
        status: 201,
        body: {
          observation: observation("o9", "Latency"),
          folder: {},
          permissions: {},
        },
      },
      "POST /api/v1/observations/o9/blocks": {
        status: 201,
        body: blockReply("live"),
      },
      "GET /api/v1/observations/o9": { status: 599, body: null },
    })
    const { user, onClose } = render()
    expect(
      await screen.findByText("No open observations. Start a new one.")
    ).toBeInTheDocument()
    expect(screen.getByLabelText("Title")).toHaveValue("Latency")
    await user.click(screen.getByRole("button", { name: "Add" }))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
    const writes = calls.filter((call) => call.method === "POST")
    expect(writes.map((call) => call.path)).toEqual([
      "/api/v1/observations",
      "/api/v1/observations/o9/blocks",
    ])
    expect(writes[0]?.body).toEqual({ title: "Latency" })
    expect(toast.success).toHaveBeenCalledWith(
      "Added to Latency",
      expect.anything()
    )
  })

  it("adds to the observation it already created instead of creating a second one", async () => {
    const calls = stubServer({
      "GET /api/v1/observations": openList(),
      "GET /api/v1/folders": foldersReply,
      "POST /api/v1/observations": {
        status: 201,
        body: {
          observation: observation("o9", "Latency"),
          folder: {},
          permissions: {},
        },
      },
      "POST /api/v1/observations/o9/blocks": {
        status: 504,
        body: { error: "timed out", code: "query_timeout" },
      },
    })
    const { user, onClose } = render()
    await user.click(await screen.findByRole("button", { name: "Add" }))
    expect(
      await screen.findByText(
        "A datasource did not answer in time, so its data could not be read. Try again."
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'The observation "Latency" was created. Add again to put the panel on it.'
      )
    ).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Add" }))

    await waitFor(() =>
      expect(
        calls.filter((call) => call.path === "/api/v1/observations/o9/blocks")
      ).toHaveLength(2)
    )
    expect(
      calls.filter(
        (call) => call.method === "POST" && call.path === "/api/v1/observations"
      )
    ).toHaveLength(1)
    expect(onClose).not.toHaveBeenCalled()
  })

  it("keeps the panel open with the sentence when the observation refuses", async () => {
    stubServer({
      "GET /api/v1/observations": openList(observation("o1", "Checkout slow")),
      "GET /api/v1/folders": foldersReply,
      "POST /api/v1/observations/o1/blocks": {
        status: 403,
        body: {
          error: "you cannot add blocks to this observation",
          code: "forbidden_role",
        },
      },
    })
    const { user, onClose } = render()
    await user.click(await screen.findByRole("combobox"))
    await user.click(
      await screen.findByRole("option", { name: "Checkout slow" })
    )
    await user.click(screen.getByRole("button", { name: "Add" }))

    expect(
      await screen.findByText("You cannot add blocks to this observation.")
    ).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
  })
})
