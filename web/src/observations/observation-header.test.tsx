import { createRef } from "react"
import { cleanup, screen, waitFor } from "@testing-library/react"
import { toast } from "sonner"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { ActionsHostContext } from "@/shell/top-row-actions"
import { renderWithProviders } from "@/test/render"

import { ObservationHeader } from "./observation-header"
import type { Block, ObservationEnvelope } from "./queries"

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

function block(kind: "text" | "panel", state: "live" | "requested"): Block {
  return {
    id: `b-${kind}-${state}`,
    observation_id: "o1",
    kind,
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
  }
}

function envelope(
  over: {
    dashboard_id?: string | null
    pinned?: boolean
    can_pin?: boolean
  },
  blocks: Block[]
): ObservationEnvelope {
  return {
    observation: {
      id: "o1",
      folder_id: "f1",
      title: "Checkout 5xx",
      state: "open",
      summary: null,
      from: null,
      to: null,
      thread_id: null,
      run_id: null,
      dashboard_id: over.dashboard_id ?? null,
      pinned: over.pinned ?? false,
      can_pin: over.can_pin ?? over.dashboard_id !== undefined,
      message_count: 0,
      url: "http://localhost/o/o1",
      created_by: author,
      created_at: "2026-11-12T10:00:00Z",
      updated_at: "2026-11-12T10:00:00Z",
      resolved_at: null,
      allowed_actions: ["move"],
      blocks,
    },
    folder: { id: "f1", title: "Incidents" },
  } as unknown as ObservationEnvelope
}

type Call = { method: string; path: string; body: unknown }

function stubServer(
  replies: Record<string, { status: number; body: unknown }>
) {
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

async function openMenu(value: ObservationEnvelope) {
  const host = document.createElement("div")
  document.body.append(host)
  const rendered = renderWithProviders(
    <ActionsHostContext value={host}>
      <ObservationHeader
        envelope={value}
        discussion={{
          shown: false,
          count: 0,
          onToggle: vi.fn(),
          toggleRef: createRef<HTMLButtonElement>(),
        }}
        onMove={vi.fn()}
        moreButtonRef={createRef<HTMLButtonElement>()}
      />
    </ActionsHostContext>,
    { route: { path: "/", url: "/" } }
  )
  await rendered.user.click(
    await screen.findByRole("button", { name: "More actions" })
  )
  return rendered
}

beforeEach(() => {
  vi.mocked(toast.success).mockClear()
  vi.mocked(toast.error).mockClear()
})

afterEach(() => {
  vi.unstubAllGlobals()
  cleanup()
  document.body.replaceChildren()
})

describe("ObservationHeader actions", () => {
  it("offers Pin and Make a dashboard when the run was about a dashboard and a panel is live", async () => {
    const calls = stubServer({
      "POST /api/v1/observations/o1/pin": {
        status: 201,
        body: { dashboard_id: "d1", annotation_id: "a1" },
      },
    })
    const { user } = await openMenu(
      envelope({ dashboard_id: "d1" }, [block("panel", "live")])
    )
    expect(
      await screen.findByRole("menuitem", { name: "Make a dashboard" })
    ).toBeTruthy()
    await user.click(screen.getByRole("menuitem", { name: "Pin to dashboard" }))
    await waitFor(() =>
      expect(calls).toEqual([
        { method: "POST", path: "/api/v1/observations/o1/pin", body: {} },
      ])
    )
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Pinned to the dashboard")
    )
  })

  it("shows a pinned observation as pinned and disabled", async () => {
    await openMenu(
      envelope({ dashboard_id: "d1", pinned: true }, [block("text", "live")])
    )
    const item = await screen.findByRole("menuitem", {
      name: "Pinned to dashboard",
    })
    expect(item.getAttribute("aria-disabled")).toBe("true")
    expect(
      screen.queryByRole("menuitem", { name: "Make a dashboard" })
    ).toBeNull()
  })

  it("leaves Pin out when the caller may not edit the dashboard", async () => {
    await openMenu(
      envelope({ dashboard_id: "d1", can_pin: false }, [block("panel", "live")])
    )
    await screen.findByRole("menuitem", { name: "Make a dashboard" })
    expect(
      screen.queryByRole("menuitem", { name: "Pin to dashboard" })
    ).toBeNull()
  })

  it("leaves both out without a dashboard to pin to and without a live panel", async () => {
    await openMenu(envelope({}, [block("panel", "requested")]))
    await screen.findByRole("menuitem", { name: "Move to folder" })
    expect(
      screen.queryByRole("menuitem", { name: "Pin to dashboard" })
    ).toBeNull()
    expect(
      screen.queryByRole("menuitem", { name: "Make a dashboard" })
    ).toBeNull()
  })
})
