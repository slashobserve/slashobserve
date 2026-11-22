import { screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { observeKeys } from "@/api/keys"
import { CanvasHostProvider, type CanvasHost } from "@/comments/canvas-host"
import { useCanvasTool } from "@/dashboards/canvas-tools"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { Block } from "./queries"
import { WholeBlockCanvas } from "./whole-block-canvas"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

const runBlock = {
  id: "b3",
  observation_id: "o1",
  kind: "run",
  state: "live",
  body: {},
} as unknown as Block

const host: CanvasHost = {
  subject: { kind: "observation", id: "o1" },
  gate: {
    canComment: true,
    agentAvailable: true,
    pinned: false,
    layoutEditing: false,
  },
  chat: { chatId: null, select: () => {} },
}

function show(onCanvas: boolean) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observeKeys.agent(), {
    available: true,
    provider: "anthropic",
    model: "claude-haiku-5-5",
    effort: null,
  })
  return renderWithProviders(
    <CanvasHostProvider value={host}>
      <WholeBlockCanvas block={runBlock} onCanvas={onCanvas}>
        <p>The run checked the checkout errors.</p>
      </WholeBlockCanvas>
    </CanvasHostProvider>,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ service_accounts: [], connected: [] }))
  )
  useCanvasTool.setState({
    tool: "comment",
    draft: null,
    openThreadId: null,
    openTurn: null,
    postedThreadId: null,
    text: "",
    leaving: null,
    detached: null,
  })
})

afterEach(() => vi.unstubAllGlobals())

describe("WholeBlockCanvas", () => {
  it("opens a whole-block comment on a run block from a click in Comment mode", async () => {
    const { user } = show(true)
    await user.click(
      await screen.findByText("The run checked the checkout errors.")
    )
    expect(useCanvasTool.getState().draft).toMatchObject({
      ownerId: "b3",
      tool: "comment",
      anchor: null,
      surface: "block:b3",
    })
    expect(
      await screen.findByRole("dialog", { name: "Comment on the canvas" })
    ).toBeInTheDocument()
  })

  it("asks about the whole block in Ask mode", async () => {
    useCanvasTool.setState({ tool: "ask" })
    const { user } = show(true)
    await user.click(
      await screen.findByText("The run checked the checkout errors.")
    )
    expect(useCanvasTool.getState().draft).toMatchObject({
      ownerId: "b3",
      tool: "ask",
      anchor: null,
    })
  })

  it("takes no click off the canvas", async () => {
    const { user } = show(false)
    await user.click(
      await screen.findByText("The run checked the checkout errors.")
    )
    expect(useCanvasTool.getState().draft).toBeNull()
  })
})
