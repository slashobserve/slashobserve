import type { ReactNode } from "react"
import { act, render, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { FakeEventSource } from "@/test/fake-event-source"

import type { ActivityEvent } from "./activity-match"
import {
  ActivityStreamProvider,
  useActivitySubscription,
  useHasActivityStream,
} from "./activity-stream"

function event(type: string, folderId: string | null): ActivityEvent {
  return {
    seq: 1,
    type,
    target: { kind: "dashboard", id: "d1" },
    dashboard_id: "d1",
    folder_id: folderId,
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ada",
    },
    data: {},
    created_at: "2026-11-25T10:00:00Z",
  } as unknown as ActivityEvent
}

function wrapper({ children }: { children: ReactNode }) {
  return <ActivityStreamProvider>{children}</ActivityStreamProvider>
}

function subscribers() {
  const all = { onEvent: vi.fn(), onGap: vi.fn() }
  const folder = { onEvent: vi.fn(), onGap: vi.fn() }
  const types = { onEvent: vi.fn(), onGap: vi.fn() }
  renderHook(
    () => {
      useActivitySubscription(all)
      useActivitySubscription({ ...folder, folderId: "f1" })
      useActivitySubscription({
        ...types,
        types: ["dashboard.created", "dashboard.deleted"],
      })
    },
    { wrapper }
  )
  return { all, folder, types }
}

beforeEach(() => {
  FakeEventSource.instances = []
  vi.stubGlobal("EventSource", FakeEventSource)
})

afterEach(() => vi.unstubAllGlobals())

describe("ActivityStreamProvider", () => {
  it("opens one unfiltered stream for every subscriber", () => {
    subscribers()
    expect(FakeEventSource.instances.map((source) => source.url)).toEqual([
      "/api/v1/activity/stream",
    ])
  })

  it("hands each event only to the subscribers whose filter it matches", () => {
    const { all, folder, types } = subscribers()
    const created = event("dashboard.created", "f1")
    const moved = event("dashboard.moved", "f2")

    act(() => {
      FakeEventSource.instances[0].emit("activity", created)
      FakeEventSource.instances[0].emit("activity", moved)
    })

    expect(all.onEvent.mock.calls).toEqual([[created], [moved]])
    expect(folder.onEvent.mock.calls).toEqual([[created]])
    expect(types.onEvent.mock.calls).toEqual([[created]])
  })

  it("tells every subscriber when events may have been missed", () => {
    const { all, folder, types } = subscribers()

    act(() => FakeEventSource.instances[0].emit("reset"))

    expect(all.onGap).toHaveBeenCalledTimes(1)
    expect(folder.onGap).toHaveBeenCalledTimes(1)
    expect(types.onGap).toHaveBeenCalledTimes(1)
  })

  it("stops handing events to a subscriber that unmounted", () => {
    const onEvent = vi.fn()
    function Subscriber() {
      useActivitySubscription({ onEvent, onGap: () => undefined })
      return null
    }
    const { rerender } = render(
      <ActivityStreamProvider>
        <Subscriber />
      </ActivityStreamProvider>
    )
    rerender(<ActivityStreamProvider>{null}</ActivityStreamProvider>)

    act(() => FakeEventSource.instances[0].emit("activity", event("x.y", null)))

    expect(onEvent).not.toHaveBeenCalled()
    expect(FakeEventSource.open()).toHaveLength(1)
  })

  it("says whether a stream is shared above", () => {
    expect(renderHook(useHasActivityStream).result.current).toBe(false)
    expect(renderHook(useHasActivityStream, { wrapper }).result.current).toBe(
      true
    )
  })
})
