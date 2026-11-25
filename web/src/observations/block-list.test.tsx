import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { BlockList } from "./block-list"
import type { Block, Observation } from "./queries"

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function block(id: string, position: number, overrides: Partial<Block> = {}) {
  return {
    id,
    observation_id: "o1",
    kind: "text",
    state: "live",
    position,
    width: "full",
    height: null,
    version: 1,
    body: { markdown: `Text ${id}` },
    snapshot_id: null,
    run_id: null,
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    allowed_actions: ["update", "remove"],
    ...overrides,
  } as Block
}

function observation(allowed: string[]): Observation {
  return { id: "o1", allowed_actions: allowed } as unknown as Observation
}

function stubLayout(blocks: Block[]) {
  const bodies: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "PUT") bodies.push(await request.clone().json())
      return new Response(JSON.stringify({ blocks }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return bodies
}

function show(blocks: Block[], allowed: string[]) {
  return renderWithProviders(
    <BlockList
      observation={observation(allowed)}
      blocks={blocks}
      canAdd
      canComment={false}
      reveal={null}
      onRevealed={() => {}}
      onAdded={() => {}}
      footerSlot={null}
    />,
    { route: { path: "/", url: "/" } }
  )
}

afterEach(() => vi.unstubAllGlobals())

describe("arranging blocks from the keyboard", () => {
  it("moves a block with the arrow keys and puts the whole order without versions", async () => {
    const blocks = [block("a", 0), block("b", 1)]
    const bodies = stubLayout([blocks[1], { ...blocks[0], position: 1 }])
    const { user } = show(blocks, ["update"])
    const handles = await screen.findAllByRole("button", {
      name: /^Move or resize Text/,
    })
    handles[0].focus()
    await user.keyboard("{ArrowDown}")
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toEqual({ blocks: [{ id: "b" }, { id: "a" }] })
  })

  it("sends one write at a time and the undone order after the first returns", async () => {
    const blocks = [block("a", 0), block("b", 1)]
    const puts: { body: unknown; release: () => void }[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.method !== "PUT") {
          return new Response(JSON.stringify({ blocks }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        }
        const body = await request.clone().json()
        await new Promise<void>((release) => puts.push({ body, release }))
        const order = (body as { blocks: { id: string }[] }).blocks.map(
          (entry) => entry.id
        )
        return new Response(
          JSON.stringify({
            blocks: order.map((id, position) => ({
              ...blocks.find((b) => b.id === id),
              position,
            })),
          }),
          { status: 200, headers: { "Content-Type": "application/json" } }
        )
      })
    )
    const { user } = show(blocks, ["update"])
    const [handle] = await screen.findAllByRole("button", {
      name: /^Move or resize Text/,
    })
    handle.focus()
    await user.keyboard("{ArrowDown}")
    await waitFor(() => expect(puts).toHaveLength(1))
    await user.keyboard("{ArrowUp}")
    expect(puts).toHaveLength(1)
    puts[0].release()
    await waitFor(() => expect(puts).toHaveLength(2))
    expect(puts[1].body).toEqual({ blocks: [{ id: "a" }, { id: "b" }] })
    puts[1].release()
  })

  it("steps the width with shift and the side arrows", async () => {
    const blocks = [block("a", 0, { width: "half" })]
    const bodies = stubLayout(blocks)
    const { user } = show(blocks, ["update"])
    const [handle] = await screen.findAllByRole("button", {
      name: /^Move or resize Text/,
    })
    handle.focus()
    await user.keyboard("{Shift>}{ArrowRight}{/Shift}")
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toEqual({
      blocks: [{ id: "a", width: "two_thirds" }],
    })
  })

  it("changes the height of a panel with shift and the up and down arrows", async () => {
    const blocks = [
      block("a", 0, { kind: "snapshot", width: "half", body: {} }),
    ]
    const bodies = stubLayout(blocks)
    const { user } = show(blocks, ["update"])
    const [handle] = await screen.findAllByRole("button", {
      name: /^Move or resize/,
    })
    handle.focus()
    await user.keyboard("{Shift>}{ArrowDown}{/Shift}")
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toEqual({ blocks: [{ id: "a", height: 260 }] })
  })

  it("offers no handle to someone who cannot edit the observation", async () => {
    stubLayout([])
    show([block("a", 0, { allowed_actions: [] })], [])
    await screen.findByText("Text a")
    expect(screen.queryByRole("button", { name: /^Move or resize/ })).toBeNull()
  })
})

describe("a requested block's keep row", () => {
  const livePanel = block("p", 0, {
    kind: "panel",
    body: {
      panel: {
        type: "timeseries",
        title: "Errors",
        datasource: "prom",
        queries: [{ query: "up" }],
      },
    },
  })
  const line =
    "Keeping it adds this block next to the one above; it replaces nothing."

  it("tells an editor that keeping a block after a live panel adds a second one", async () => {
    stubLayout([])
    show(
      [
        livePanel,
        block("s", 1, {
          state: "requested",
          allowed_actions: ["keep", "discard"],
        }),
      ],
      ["update"]
    )
    await screen.findByText("Text s")
    expect(screen.getByText(line)).toBeTruthy()
  })

  it("says nothing more after a text block or to the person who asked", async () => {
    stubLayout([])
    show(
      [
        block("t", 0),
        block("s", 1, {
          state: "requested",
          allowed_actions: ["keep", "discard"],
        }),
        { ...livePanel, id: "q", position: 2 },
        block("r", 3, { state: "requested", allowed_actions: ["discard"] }),
      ],
      ["update"]
    )
    await screen.findByText("Text r")
    expect(screen.queryByText(line)).toBeNull()
  })
})
