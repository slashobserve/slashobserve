import { createElement, type ReactNode } from "react"
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { observationKeys, threadKeys } from "@/api/keys"
import { testQueryClient } from "@/test/render"

import { BlockVersionConflict } from "./errors"
import {
  keepKnownBlocks,
  observationQuery,
  observationsInfiniteQuery,
  replaceBlock,
  useAddBlock,
  useAddBlockTo,
  useCreateObservation,
  useDiscardBlock,
  useKeepBlock,
  useMoveObservation,
  useObservation,
  useObservationLive,
  useRemoveBlock,
  useReopenObservation,
  useResolveObservation,
  useUpdateBlock,
  useUpdateObservation,
  withoutBlock,
  type Block,
  type ObservationEnvelope,
} from "./queries"

const author = {
  kind: "user" as const,
  id: "u1",
  name: "Ada",
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function block(id: string, position: number, version = 1): Block {
  return {
    id,
    observation_id: "o1",
    kind: "text",
    state: "live",
    position,
    width: "full",
    version,
    body: { markdown: id },
    snapshot_id: null,
    run_id: null,
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
    allowed_actions: ["update", "remove"],
  }
}

function envelope(
  blocks: Block[] | undefined,
  overrides: Partial<ObservationEnvelope["observation"]> = {}
): ObservationEnvelope {
  return {
    observation: {
      id: "o1",
      folder_id: "f1",
      title: "Checkout slow",
      state: "open",
      summary: null,
      from: null,
      to: null,
      thread_id: "t1",
      run_id: null,
      url: "http://localhost/o/o1",
      created_by: author,
      created_at: "2026-11-12T10:00:00Z",
      updated_at: "2026-11-12T10:00:00Z",
      resolved_at: null,
      allowed_actions: ["update"],
      blocks,
      ...overrides,
    },
  } as ObservationEnvelope
}

describe("replaceBlock", () => {
  it("replaces the block with the same id and leaves the others untouched", () => {
    const a = block("a", 0)
    const b = block("b", 1)
    const c = block("c", 2)
    const newer = block("b", 1, 2)
    const result = replaceBlock(envelope([a, b, c]), newer)
    expect(result.observation.blocks).toEqual([a, newer, c])
    expect(result.observation.blocks?.[0]).toBe(a)
    expect(result.observation.blocks?.[2]).toBe(c)
  })

  it("does not add a block the page does not have", () => {
    const blocks = [block("a", 0)]
    expect(
      replaceBlock(envelope(blocks), block("z", 1)).observation.blocks
    ).toEqual(blocks)
  })

  it("leaves an observation read without blocks alone", () => {
    const bare = envelope(undefined)
    expect(replaceBlock(bare, block("a", 0))).toBe(bare)
  })
})

describe("withoutBlock", () => {
  it("drops only the named block", () => {
    const a = block("a", 0)
    const b = block("b", 1)
    expect(withoutBlock(envelope([a, b]), "a").observation.blocks).toEqual([b])
  })
})

describe("keepKnownBlocks", () => {
  it("keeps the cached blocks when the new answer has none", () => {
    const blocks = [block("a", 0)]
    const merged = keepKnownBlocks(envelope(blocks), envelope(undefined))
    expect(merged.observation.blocks).toEqual(blocks)
  })

  it("takes the new blocks when the answer carries them", () => {
    const fresh = [block("a", 0, 3)]
    const merged = keepKnownBlocks(envelope([block("a", 0)]), envelope(fresh))
    expect(merged.observation.blocks).toEqual(fresh)
  })

  it("takes the answer as it is when nothing is cached", () => {
    const next = envelope(undefined)
    expect(keepKnownBlocks(undefined, next)).toBe(next)
  })
})

type Reply = { status: number; body?: unknown }
type Call = { method: string; path: string; search: string; body: unknown }

function stubServer(replies: Record<string, Reply>) {
  const calls: Call[] = []
  const fetch = vi.fn(async (request: Request) => {
    const url = new URL(request.url)
    const text = await request.clone().text()
    calls.push({
      method: request.method,
      path: url.pathname,
      search: url.search,
      body: text === "" ? undefined : JSON.parse(text),
    })
    const reply = replies[`${request.method} ${url.pathname}`]
    if (reply === undefined) return new Response(null, { status: 599 })
    return new Response(
      reply.body === undefined ? null : JSON.stringify(reply.body),
      {
        status: reply.status,
        headers: { "Content-Type": "application/json" },
      }
    )
  })
  vi.stubGlobal("fetch", fetch)
  return calls
}

function wrapperFor(queryClient: QueryClient) {
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
}

function seededClient(
  blocks: Block[] | undefined = [block("a", 0), block("b", 1)]
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(observationKeys.one("o1"), envelope(blocks))
  queryClient.setQueryData(observationKeys.list({}, 20), { pages: [] })
  return queryClient
}

function cachedBlocks(queryClient: QueryClient) {
  return queryClient.getQueryData<ObservationEnvelope>(
    observationKeys.one("o1")
  )?.observation.blocks
}

function isInvalidated(queryClient: QueryClient, key: readonly unknown[]) {
  return queryClient.getQueryState(key)?.isInvalidated === true
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("observation reads", () => {
  it("reads one observation with its blocks", async () => {
    const served = envelope([block("a", 0)])
    const calls = stubServer({
      "GET /api/v1/observations/o1": { status: 200, body: served },
    })
    const queryClient = testQueryClient()
    const { result } = renderHook(() => useObservation("o1"), {
      wrapper: wrapperFor(queryClient),
    })
    await waitFor(() => expect(result.current.data).toEqual(served))
    expect(calls).toHaveLength(1)
    expect(await queryClient.fetchQuery(observationQuery("o1"))).toEqual(served)
  })

  it("asks for a page with the state filter and the cursor", async () => {
    const calls = stubServer({
      "GET /api/v1/observations": {
        status: 200,
        body: { observations: [], next_cursor: "c2" },
      },
    })
    const queryClient = testQueryClient()
    const options = observationsInfiniteQuery({ state: "resolved" }, 8)
    const page = await queryClient.fetchInfiniteQuery(options)
    expect(page.pages[0]?.next_cursor).toBe("c2")
    expect(new URLSearchParams(calls[0]?.search).get("state")).toBe("resolved")
    expect(new URLSearchParams(calls[0]?.search).get("limit")).toBe("8")
    expect(new URLSearchParams(calls[0]?.search).has("cursor")).toBe(false)
  })
})

describe("observation writes", () => {
  it("files the new observation in the cache and marks the lists stale", async () => {
    const created = envelope([block("a", 0)])
    const calls = stubServer({
      "POST /api/v1/observations": { status: 201, body: created },
    })
    const queryClient = seededClient()
    const { result } = renderHook(() => useCreateObservation(), {
      wrapper: wrapperFor(queryClient),
    })
    await act(() =>
      result.current.mutateAsync({ title: "Checkout slow", folder_id: "f1" })
    )
    expect(calls[0]?.body).toEqual({ title: "Checkout slow", folder_id: "f1" })
    expect(cachedBlocks(queryClient)).toEqual([block("a", 0)])
    expect(isInvalidated(queryClient, observationKeys.list({}, 20))).toBe(true)
  })

  it.each([
    {
      name: "update",
      run: (hooks: AllHooks) => hooks.update.mutateAsync({ title: "New" }),
      method: "PATCH",
      path: "/api/v1/observations/o1",
      body: { title: "New" },
    },
    {
      name: "move",
      run: (hooks: AllHooks) => hooks.move.mutateAsync({ folder_id: "f2" }),
      method: "POST",
      path: "/api/v1/observations/o1/move",
      body: { folder_id: "f2" },
    },
    {
      name: "resolve",
      run: (hooks: AllHooks) => hooks.resolve.mutateAsync({ summary: "Done" }),
      method: "POST",
      path: "/api/v1/observations/o1/resolve",
      body: { summary: "Done" },
    },
    {
      name: "reopen",
      run: (hooks: AllHooks) => hooks.reopen.mutateAsync(undefined),
      method: "POST",
      path: "/api/v1/observations/o1/reopen",
      body: undefined,
    },
  ])(
    "$name writes the answer into the cache, keeps the blocks and marks the lists stale",
    async ({ run, method, path, body }) => {
      const answer = envelope(undefined, { title: "Answer" })
      const calls = stubServer({
        [`${method} ${path}`]: { status: 200, body: answer },
      })
      const queryClient = seededClient()
      const { result } = renderHook(() => useObservationHooks(), {
        wrapper: wrapperFor(queryClient),
      })
      await act(() => run(result.current))
      expect(calls[0]?.body).toEqual(body)
      const cached = queryClient.getQueryData<ObservationEnvelope>(
        observationKeys.one("o1")
      )
      expect(cached?.observation.title).toBe("Answer")
      expect(cached?.observation.blocks).toHaveLength(2)
      expect(isInvalidated(queryClient, observationKeys.list({}, 20))).toBe(
        true
      )
    }
  )
})

function useObservationHooks() {
  return {
    update: useUpdateObservation("o1"),
    move: useMoveObservation("o1"),
    resolve: useResolveObservation("o1"),
    reopen: useReopenObservation("o1"),
  }
}
type AllHooks = ReturnType<typeof useObservationHooks>

describe("block writes", () => {
  it("adds a block and refetches the page for the shifted positions", async () => {
    const calls = stubServer({
      "POST /api/v1/observations/o1/blocks": {
        status: 201,
        body: { block: block("c", 1) },
      },
    })
    const queryClient = seededClient()
    const { result } = renderHook(() => useAddBlock("o1"), {
      wrapper: wrapperFor(queryClient),
    })
    await act(() =>
      result.current.mutateAsync({
        kind: "text",
        body: { markdown: "c" },
        position: 1,
      })
    )
    expect(calls[0]?.body).toEqual({
      kind: "text",
      body: { markdown: "c" },
      position: 1,
    })
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(true)
  })

  it("adds a block to the observation named with the call and refetches only that one", async () => {
    const calls = stubServer({
      "POST /api/v1/observations/o2/blocks": {
        status: 201,
        body: { block: { ...block("c", 0), observation_id: "o2" } },
      },
    })
    const queryClient = seededClient()
    queryClient.setQueryData(observationKeys.one("o2"), envelope([]))
    const { result } = renderHook(() => useAddBlockTo(), {
      wrapper: wrapperFor(queryClient),
    })
    const panel = { panel: { type: "logs", title: "Logs" } }
    await act(() =>
      result.current.mutateAsync({
        observationId: "o2",
        kind: "panel",
        body: panel,
      })
    )
    expect(calls).toHaveLength(1)
    expect(calls[0]?.path).toBe("/api/v1/observations/o2/blocks")
    expect(calls[0]?.body).toEqual({ kind: "panel", body: panel })
    expect(isInvalidated(queryClient, observationKeys.one("o2"))).toBe(true)
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(false)
    expect(isInvalidated(queryClient, observationKeys.list({}, 20))).toBe(true)
  })

  it("sends the version with an update and shows the saved block", async () => {
    const saved = block("a", 0, 2)
    const calls = stubServer({
      "PATCH /api/v1/blocks/a": { status: 200, body: { block: saved } },
    })
    const queryClient = seededClient()
    const { result } = renderHook(() => useUpdateBlock("o1"), {
      wrapper: wrapperFor(queryClient),
    })
    await act(() =>
      result.current.mutateAsync({
        blockId: "a",
        version: 1,
        body: { markdown: "changed" },
      })
    )
    expect(calls[0]?.body).toEqual({
      version: 1,
      body: { markdown: "changed" },
    })
    expect(cachedBlocks(queryClient)).toEqual([saved, block("b", 1)])
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(true)
  })

  it("shows the latest block and rejects when someone else changed it", async () => {
    const latest = block("a", 0, 5)
    stubServer({
      "PATCH /api/v1/blocks/a": {
        status: 409,
        body: {
          error: "version conflict",
          code: "version_conflict",
          current: latest,
        },
      },
    })
    const queryClient = seededClient()
    const { result } = renderHook(() => useUpdateBlock("o1"), {
      wrapper: wrapperFor(queryClient),
    })
    await act(async () => {
      await expect(
        result.current.mutateAsync({ blockId: "a", version: 1, width: "half" })
      ).rejects.toBeInstanceOf(BlockVersionConflict)
    })
    expect(cachedBlocks(queryClient)).toEqual([latest, block("b", 1)])
  })

  it("removes a block with the version in the query and drops it from the page", async () => {
    const calls = stubServer({
      "DELETE /api/v1/blocks/a": { status: 204 },
    })
    const queryClient = seededClient()
    const { result } = renderHook(() => useRemoveBlock("o1"), {
      wrapper: wrapperFor(queryClient),
    })
    await act(() => result.current.mutateAsync({ blockId: "a", version: 3 }))
    expect(calls[0]?.search).toBe("?version=3")
    expect(cachedBlocks(queryClient)).toEqual([block("b", 1)])
  })

  it("keeps a requested block at the version that was read", async () => {
    const kept = block("a", 0, 2)
    const calls = stubServer({
      "POST /api/v1/blocks/a/keep": { status: 200, body: { block: kept } },
    })
    const queryClient = seededClient()
    const { result } = renderHook(() => useKeepBlock("o1"), {
      wrapper: wrapperFor(queryClient),
    })
    await act(() => result.current.mutateAsync({ blockId: "a", version: 1 }))
    expect(calls[0]?.body).toEqual({ version: 1 })
    expect(cachedBlocks(queryClient)?.[0]).toEqual(kept)
  })

  it("discards a requested block and drops it from the page", async () => {
    stubServer({ "POST /api/v1/blocks/b/discard": { status: 204 } })
    const queryClient = seededClient()
    const { result } = renderHook(() => useDiscardBlock("o1"), {
      wrapper: wrapperFor(queryClient),
    })
    await act(() => result.current.mutateAsync({ blockId: "b" }))
    expect(cachedBlocks(queryClient)).toEqual([block("a", 0)])
  })
})

type Listener = (event: { data?: string }) => void

class FakeEventSource {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2
  static instances: FakeEventSource[] = []
  readyState = FakeEventSource.OPEN
  onerror: (() => void) | null = null
  closed = false
  private listeners = new Map<string, Listener[]>()

  readonly url: string

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(name: string, listener: Listener) {
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), listener])
  }

  close() {
    this.closed = true
    this.readyState = FakeEventSource.CLOSED
  }

  emit(name: string, data?: unknown) {
    for (const listener of this.listeners.get(name) ?? []) {
      listener({ data: data === undefined ? undefined : JSON.stringify(data) })
    }
  }

  fail(readyState: number) {
    this.readyState = readyState
    this.onerror?.()
  }
}

function activity(kind: string, id: string) {
  return {
    seq: 1,
    type: "observation.updated",
    target: { kind, id },
    dashboard_id: null,
    folder_id: "f1",
    actor: author,
    data: {},
    created_at: "2026-11-12T10:00:00Z",
  }
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", {
    value: hidden,
    configurable: true,
  })
  document.dispatchEvent(new Event("visibilitychange"))
}

describe("useObservationLive", () => {
  const live = {
    id: "o1",
    folder_id: "f1",
    thread_id: "t1",
    blocks: [{ run_id: "r1" }] as Block[],
  }

  beforeEach(() => {
    FakeEventSource.instances = []
    vi.stubGlobal("EventSource", FakeEventSource)
  })

  afterEach(() => {
    vi.useRealTimers()
    setHidden(false)
  })

  function mount() {
    const queryClient = seededClient()
    queryClient.setQueryData(threadKeys.list("observation:o1"), { pages: [] })
    renderHook(() => useObservationLive(live), {
      wrapper: wrapperFor(queryClient),
    })
    return queryClient
  }

  it("listens to the observation's folder", () => {
    mount()
    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.instances[0]?.url).toBe(
      "/api/v1/activity/stream?folder_id=f1"
    )
  })

  it.each([
    ["the observation", "observation", "o1", "page"],
    ["a run block of the page", "run", "r1", "page"],
    ["the discussion thread", "thread", "t1", "discussion"],
  ])("refreshes only what %s changes", (_, kind, id, refreshed) => {
    const queryClient = mount()
    act(() =>
      FakeEventSource.instances[0]?.emit("activity", activity(kind, id))
    )
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(
      refreshed === "page"
    )
    expect(isInvalidated(queryClient, threadKeys.list("observation:o1"))).toBe(
      refreshed === "discussion"
    )
  })

  it("ignores events about other things and malformed frames", () => {
    const queryClient = mount()
    const source = FakeEventSource.instances[0]
    act(() => {
      source?.emit("activity", activity("observation", "o2"))
      source?.emit("activity", activity("thread", "t2"))
      source?.emit("activity", activity("run", "r2"))
    })
    act(() => {
      source?.emit("activity")
    })
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(false)
    expect(isInvalidated(queryClient, threadKeys.list("observation:o1"))).toBe(
      false
    )
  })

  it("refreshes the page and the discussion when events may have been missed", () => {
    const queryClient = mount()
    act(() => FakeEventSource.instances[0]?.emit("reset"))
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(true)
    expect(isInvalidated(queryClient, threadKeys.list("observation:o1"))).toBe(
      true
    )
  })

  it("reconnects after the source closes and refreshes once the new stream is ready", () => {
    vi.useFakeTimers()
    const queryClient = mount()
    const first = FakeEventSource.instances[0]
    act(() => first?.fail(FakeEventSource.CLOSED))
    expect(first?.closed).toBe(true)
    act(() => {
      vi.advanceTimersByTime(1_000)
    })
    expect(FakeEventSource.instances).toHaveLength(2)
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(false)
    act(() => FakeEventSource.instances[1]?.emit("ready"))
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(true)
  })

  it("leaves a connecting source to the browser's own reconnect", () => {
    vi.useFakeTimers()
    mount()
    act(() => FakeEventSource.instances[0]?.fail(FakeEventSource.CONNECTING))
    act(() => {
      vi.advanceTimersByTime(60_000)
    })
    expect(FakeEventSource.instances).toHaveLength(1)
  })

  it("closes while the tab is hidden and refreshes when it returns", () => {
    const queryClient = mount()
    const first = FakeEventSource.instances[0]
    act(() => setHidden(true))
    expect(first?.closed).toBe(true)
    act(() => setHidden(false))
    expect(FakeEventSource.instances).toHaveLength(2)
    act(() => FakeEventSource.instances[1]?.emit("ready"))
    expect(isInvalidated(queryClient, observationKeys.one("o1"))).toBe(true)
  })
})
