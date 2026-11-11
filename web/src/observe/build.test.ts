import { QueryClient, QueryObserver } from "@tanstack/react-query"
import { describe, expect, it, vi } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"
import { observeKeys } from "@/api/keys"

import {
  buildGone,
  buildStatusText,
  cancelErrorText,
  continueErrorText,
  continueNoteError,
  newPanelIds,
  readBuild,
  saveBuildErrorText,
  savedDashboardId,
  answeredRequestId,
  type BuildProgress,
} from "./build"
import type { ObserveRequest } from "./queries"

function progress(overrides: Partial<BuildProgress> = {}): BuildProgress {
  return {
    saves: 3,
    panels: 12,
    planned_panels: 40,
    title: "Checkout",
    updated_at: "2026-11-03T10:00:00Z",
    ...overrides,
  }
}

describe("buildStatusText", () => {
  it.each([
    ["queued", null, null, "Waiting for the agent"],
    ["running", null, null, "Waiting for the agent"],
    ["running", progress(), null, "Building, 12 of 40 panels"],
    ["running", progress({ panels: 40 }), null, "Building, 40 of 40 panels"],
    ["running", progress({ panels: 45 }), null, "Building, 45 panels"],
    [
      "running",
      progress({ planned_panels: 0, panels: 12 }),
      null,
      "Building, 12 panels",
    ],
    [
      "running",
      progress({ planned_panels: 0, panels: 1 }),
      null,
      "Building, 1 panel",
    ],
    [
      "failed",
      progress({ panels: 37, planned_panels: 114 }),
      "no_progress",
      "Stopped at 37 of 114 panels",
    ],
    [
      "failed",
      progress({ panels: 37, planned_panels: 114 }),
      "canceled",
      "Canceled at 37 of 114 panels",
    ],
    [
      "failed",
      progress({ planned_panels: 0, panels: 41 }),
      "no_progress",
      "Stopped with 41 panels",
    ],
    [
      "failed",
      progress({ planned_panels: 0, panels: 1 }),
      "canceled",
      "Canceled with 1 panel",
    ],
    [
      "failed",
      progress({ panels: 45, planned_panels: 40 }),
      "no_progress",
      "Stopped with 45 panels",
    ],
    [
      "failed",
      progress({ panels: 0 }),
      "no_progress",
      "Stopped before anything was built",
    ],
    ["failed", null, "provider_error", "Stopped before anything was built"],
    ["succeeded", null, null, "Finished"],
  ] as const)(
    "%s with %j and %s reads %s",
    (status, build, failureCode, text) => {
      expect(buildStatusText({ status, build, failureCode })).toBe(text)
    }
  )
})

describe("buildStatusText queue, cancel and continue", () => {
  it.each([
    ["queued", null, null, 3, false, false, "Queued, position 3"],
    ["queued", null, null, 1, false, false, "Queued, position 1"],
    ["queued", null, null, null, false, false, "Waiting for the agent"],
    ["queued", null, null, 2, true, false, "Canceling"],
    ["running", progress(), null, null, true, false, "Canceling"],
    ["running", null, null, null, true, false, "Canceling"],
    ["running", progress(), null, 4, false, false, "Building, 12 of 40 panels"],
    [
      "failed",
      progress(),
      "no_progress",
      null,
      false,
      true,
      "Continued in a new run",
    ],
    [
      "failed",
      progress(),
      "canceled",
      null,
      true,
      false,
      "Canceled at 12 of 40 panels",
    ],
  ] as const)(
    "%s with %j, position %s, canceling %s, continued %s reads %s",
    (status, build, failureCode, queuePosition, canceling, continued, text) => {
      expect(
        buildStatusText({
          status,
          build,
          failureCode,
          queuePosition,
          canceling,
          continued,
        })
      ).toBe(text)
    }
  )
})

describe("newPanelIds", () => {
  const before = {
    panels: [
      { id: "row_a", panels: [{ id: "a1" }, { id: "a2" }] },
      { id: "solo" },
    ],
  }

  it("lists nothing when nothing was added", () => {
    expect(newPanelIds(before, before).size).toBe(0)
  })

  it("lists a new row and its children", () => {
    const next = {
      panels: [...before.panels, { id: "row_b", panels: [{ id: "b1" }] }],
    }
    expect([...newPanelIds(before, next)].sort()).toEqual(["b1", "row_b"])
  })

  it("lists children added to an existing row", () => {
    const next = {
      panels: [
        { id: "row_a", panels: [{ id: "a1" }, { id: "a2" }, { id: "a3" }] },
        { id: "solo" },
      ],
    }
    expect([...newPanelIds(before, next)]).toEqual(["a3"])
  })

  it("lists a panel that moved into a row as unchanged", () => {
    const next = {
      panels: [
        { id: "row_a", panels: [{ id: "a1" }, { id: "a2" }, { id: "solo" }] },
      ],
    }
    expect(newPanelIds(before, next).size).toBe(0)
  })

  it("ignores panels that were removed", () => {
    expect(newPanelIds(before, { panels: [{ id: "solo" }] }).size).toBe(0)
  })
})

describe("saveBuildErrorText", () => {
  it.each([
    [
      "version_conflict",
      "Someone saved a newer version after this build started, so it can't be saved. Open the dashboard and ask again.",
    ],
    ["forbidden", "You need Editor access to save it."],
    ["forbidden_role", "You need Editor access to save it."],
    ["build_empty", "Nothing was built yet."],
    ["build_running", "The agent is still working."],
    ["internal", "The agent is still working."],
  ])("explains %s", (code, text) => {
    expect(saveBuildErrorText(code, "the agent is still working")).toBe(text)
  })

  it("falls back when there is no message", () => {
    expect(saveBuildErrorText("internal")).toBe("The build could not be saved.")
  })
})

describe("continueNoteError", () => {
  it.each([
    ["", null],
    ["   \n ", null],
    ["a".repeat(4000), null],
    [`  ${"a".repeat(4000)}  `, null],
    ["a".repeat(4001), "Use at most 4000 characters."],
    ["\u{1F600}".repeat(4000), null],
    ["\u{1F600}".repeat(4001), "Use at most 4000 characters."],
  ])("checks a note of %#", (raw, error) => {
    expect(continueNoteError(raw)).toBe(error)
  })
})

describe("cancelErrorText", () => {
  it.each([
    ["forbidden", "Only the person who asked can do this."],
    ["request_finished", "The run already finished."],
    ["internal", "The server broke."],
  ])("explains %s", (code, text) => {
    expect(cancelErrorText(code, "the server broke")).toBe(text)
  })

  it("falls back when there is no message", () => {
    expect(cancelErrorText("internal")).toBe("The run could not be canceled.")
  })
})

describe("continueErrorText", () => {
  it.each([
    [
      "version_conflict",
      "Someone saved a newer version after this build started, so it can't be continued. Open the dashboard and ask again.",
    ],
    ["forbidden", "Only the person who asked can do this."],
    ["not_continuable", "This build can no longer be continued."],
    ["invalid_request", "Note must be short."],
  ])("explains %s", (code, text) => {
    expect(continueErrorText(code, "note must be short")).toBe(text)
  })

  it("falls back when there is no message", () => {
    expect(continueErrorText("internal")).toBe(
      "The build could not be continued."
    )
  })
})

type RequestState = Pick<
  ObserveRequest,
  "status" | "build_saved_at" | "dashboard_id"
>

const running: RequestState = {
  status: "running",
  build_saved_at: null,
  dashboard_id: "d1",
}
const succeeded: RequestState = {
  status: "succeeded",
  build_saved_at: null,
  dashboard_id: "d1",
}

const buildNotFound = new ApiError(404, "not_found", "build not found")

describe("savedDashboardId", () => {
  it.each<[string, RequestState, string | null]>([
    ["a running build", running, null],
    ["a finished run", succeeded, "d1"],
    [
      "a stopped build saved by the requester",
      {
        status: "failed",
        build_saved_at: "2026-11-06T10:00:00Z",
        dashboard_id: "d2",
      },
      "d2",
    ],
    [
      "a stopped build not saved",
      { status: "failed", build_saved_at: null, dashboard_id: "d1" },
      null,
    ],
    [
      "a finished run without a dashboard",
      { ...succeeded, dashboard_id: null },
      null,
    ],
  ])("%s", (_name, request, expected) => {
    expect(savedDashboardId(request)).toBe(expected)
  })
})

describe("answeredRequestId", () => {
  const finding = { id: "f1" } as ObserveRequest["finding"]
  it.each<
    [string, Pick<ObserveRequest, "id" | "status" | "finding">, string | null]
  >([
    ["an answered request", { id: "r1", status: "succeeded", finding }, "r1"],
    [
      "a request that built a dashboard",
      { id: "r1", status: "succeeded", finding: null },
      null,
    ],
    ["a running request", { id: "r1", status: "running", finding: null }, null],
    ["a failed request", { id: "r1", status: "failed", finding: null }, null],
  ])("%s", (_name, request, expected) => {
    expect(answeredRequestId(request)).toBe(expected)
  })
})

describe("buildGone", () => {
  it.each<[string, unknown, boolean]>([
    ["a build or request 404", buildNotFound, true],
    [
      "a panel the build does not have",
      new ApiError(404, "panel_not_found", "panel not found"),
      false,
    ],
    ["a server error", new ApiError(500, "internal", "boom"), false],
    ["a network failure", new NetworkError("offline"), false],
  ])("%s", (_name, error, expected) => {
    expect(buildGone(error)).toBe(expected)
  })
})

function deferred<T>() {
  let resolve: (value: T) => void = () => {}
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

async function clientWithRequest(answers: (() => Promise<RequestState>)[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  let calls = 0
  const queryFn = () => answers[Math.min(calls++, answers.length - 1)]()
  await client.fetchQuery({ queryKey: observeKeys.request("r1"), queryFn })
  return { client, requestCalls: () => calls }
}

function rejectWith(error: unknown) {
  return () => Promise.reject(error)
}

describe("readBuild", () => {
  it.each<[string, unknown]>([
    [
      "a panel the build does not have",
      new ApiError(404, "panel_not_found", "panel not found"),
    ],
    ["a server error", new ApiError(500, "internal", "boom")],
    ["a network failure", new NetworkError("offline")],
  ])(
    "passes %s through without asking for the request",
    async (_name, error) => {
      const { client, requestCalls } = await clientWithRequest([
        () => Promise.resolve(running),
      ])
      await expect(
        readBuild(client, "r1", new AbortController().signal, rejectWith(error))
      ).rejects.toBe(error)
      expect(requestCalls()).toBe(1)
    }
  )

  it("refetches the request at once and keeps the 404 while the run goes on", async () => {
    const { client, requestCalls } = await clientWithRequest([
      () => Promise.resolve(running),
    ])
    await expect(
      readBuild(
        client,
        "r1",
        new AbortController().signal,
        rejectWith(buildNotFound)
      )
    ).rejects.toBe(buildNotFound)
    expect(requestCalls()).toBe(2)
  })

  it("keeps the last build while the request refetch is pending and after the build became a version", async () => {
    const refetch = deferred<RequestState>()
    const { client, requestCalls } = await clientWithRequest([
      () => Promise.resolve(running),
      () => refetch.promise,
    ])
    let reads = 0
    const observer = new QueryObserver(client, {
      queryKey: observeKeys.build("r1", 1),
      queryFn: ({ client, signal }) =>
        readBuild(client, "r1", signal, () =>
          reads++ === 0
            ? Promise.resolve("first")
            : Promise.reject(buildNotFound)
        ),
    })
    const unsubscribe = observer.subscribe(() => {})
    await vi.waitFor(() =>
      expect(observer.getCurrentResult().data).toBe("first")
    )

    void observer.refetch()
    await vi.waitFor(() => expect(requestCalls()).toBe(2))
    expect(observer.getCurrentResult()).toMatchObject({
      data: "first",
      error: null,
      fetchStatus: "fetching",
    })

    refetch.resolve(succeeded)
    await vi.waitFor(() =>
      expect(client.getQueryData(observeKeys.request("r1"))).toEqual(succeeded)
    )
    await new Promise((settle) => setTimeout(settle, 10))
    expect(observer.getCurrentResult()).toMatchObject({
      data: "first",
      error: null,
      fetchStatus: "fetching",
    })

    unsubscribe()
    await vi.waitFor(() =>
      expect(client.getQueryState(observeKeys.build("r1", 1))).toMatchObject({
        status: "success",
        data: "first",
        error: null,
        fetchStatus: "idle",
      })
    )
  })

  it("shares one request refetch between the 404s of a burst", async () => {
    const refetch = deferred<RequestState>()
    const { client, requestCalls } = await clientWithRequest([
      () => Promise.resolve(running),
      () => refetch.promise,
    ])
    const reads = [1, 2, 3].map(() =>
      readBuild(
        client,
        "r1",
        new AbortController().signal,
        rejectWith(buildNotFound)
      )
    )
    await vi.waitFor(() => expect(requestCalls()).toBe(2))
    refetch.resolve(running)
    for (const read of reads) await expect(read).rejects.toBe(buildNotFound)
    expect(requestCalls()).toBe(2)
  })
})
