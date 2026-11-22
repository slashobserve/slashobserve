import { screen, waitFor } from "@testing-library/react"
import { useState } from "react"
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"

import { observeKeys, threadKeys } from "@/api/keys"
import type {
  Message,
  Thread,
  ThreadWithMessages,
} from "@/comments/thread-queries"
import type { Finding } from "@/findings/queries"
import type { ObserveRequest } from "@/observe/queries"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { noModelReviseText, pinnedText, type AgentGate } from "./delivery"
import { ThreadView } from "./thread-view"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  useRouteContext: () => ({ principal: { id: "u1", role: "editor" } }),
}))

vi.mock("@/findings/evidence-panel", () => ({
  EvidencePanel: ({ panel }: { panel: { title: string } }) => (
    <article aria-label={panel.title} />
  ),
}))

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

const target = "dashboard:d1"
const runId = "7a1d2b3c-4d5e-4f60-8a9b-0c1d2e3f4a5b"

const ada: Message["actor"] = {
  kind: "user",
  id: "u1",
  name: "Ada",
  origin: "web",
  client: null,
  run_id: null,
  label: "Ada",
}

const agent: Message["actor"] = {
  ...ada,
  run_id: runId,
  label: "Observe Agent",
}

function thread(patch: Partial<Thread> = {}): Thread {
  return {
    id: "t1",
    target: { kind: "dashboard", id: "d1" },
    dashboard_id: "d1",
    version: 1,
    panel_id: null,
    panel_present: true,
    status: "open",
    allowed_actions: ["reply", "resolve"],
    unread: false,
    created_at: "2026-11-06T09:00:00Z",
    updated_at: "2026-11-06T09:00:00Z",
    resolved_at: null,
    resolved_actor: null,
    resolved_by_version: null,
    revision: null,
    anchor: null,
    block_id: null,
    agent: { kind: "builtin", id: null },
    agent_off: false,
    ...patch,
  } as Thread
}

function message(id: string, patch: Partial<Message> = {}): Message {
  return {
    id,
    thread_id: "t1",
    body: id,
    actor: ada,
    created_at: "2026-11-06T09:00:00Z",
    updated_at: "2026-11-06T09:00:00Z",
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: null,
    reply_to: null,
    ...patch,
  }
}

const question = message("t1", {
  body: "Why did checkout errors rise?",
  run: { id: runId, status: "running", assigned_kind: "builtin" },
})

const reply = message("m2", {
  body: "Answered.",
  actor: agent,
  created_at: "2026-11-06T09:02:00Z",
})

function finding(patch: Partial<Finding> = {}): Finding {
  return {
    id: "f1",
    observation_id: null,
    agent_request_id: runId,
    dashboard_id: "d1",
    folder_id: null,
    ask_on: null,
    restricted: false,
    title: "Checkout errors rose",
    markdown: "Errors **doubled** after the deploy.",
    evidence: [
      { id: "e1", type: "timeseries", title: "Error rate" },
    ] as unknown as Finding["evidence"],
    from: "2026-11-06T08:00:00Z",
    to: "2026-11-06T09:00:00Z",
    annotation_id: null,
    created_by: { kind: "user", id: "u1", name: "Ada" },
    actor: agent,
    created_at: "2026-11-06T09:02:00Z",
    url: null,
    suggested_change: "Add a p99 latency panel",
    observe_worthy: false,
    ...patch,
  }
}

function answered(
  patch: Partial<Finding> = {},
  canSave = true,
  actor: Message["actor"] = agent
): Message {
  return {
    ...reply,
    actor,
    answer: { finding: finding(patch), can_save: canSave },
  }
}

function request(patch: Partial<ObserveRequest>): ObserveRequest {
  return {
    id: runId,
    status: "succeeded",
    state: "succeeded",
    requested_by: { kind: "user", id: "u1", name: "Ada" },
    finding: null,
    ...patch,
  } as Partial<ObserveRequest> as ObserveRequest
}

type Route = { status: number; body: unknown }

function stubFetch(routes: Record<string, Route>) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async (input) => {
      const key = `${input.method} ${new URL(input.url).pathname}`
      const route = routes[key] ?? {
        status: 404,
        body: { error: { code: "not_found", message: "not found" } },
      }
      return new Response(JSON.stringify(route.body), {
        status: route.status,
        headers: { "Content-Type": "application/json" },
      })
    }
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function Harness({
  initial,
  later,
  gate = { kind: "allowed" },
  on = target,
}: {
  initial: ThreadWithMessages
  later?: Message
  gate?: AgentGate
  on?: string
}) {
  const [entry, setEntry] = useState(initial)
  return (
    <ol>
      {later === undefined ? null : (
        <button
          type="button"
          onClick={() =>
            setEntry({ ...entry, messages: [...entry.messages, later] })
          }
        >
          Deliver the later message
        </button>
      )}
      <ThreadView
        entry={entry}
        target={on}
        panelTitle={null}
        gate={gate}
        clientName="Claude Code"
        onReplyHere={() => {}}
      />
    </ol>
  )
}

function show(
  entry: ThreadWithMessages,
  seed?: (client: ReturnType<typeof testQueryClient>) => void,
  later?: Message,
  gate?: AgentGate,
  on?: string
) {
  const queryClient = testQueryClient()
  seed?.(queryClient)
  const harness = <Harness initial={entry} later={later} gate={gate} on={on} />
  const view = renderWithProviders(harness, {
    queryClient,
    route: { path: "/", url: "/" },
  })
  return { ...view, queryClient }
}

afterEach(() => {
  vi.unstubAllGlobals()
  toasts.success.mockReset()
  toasts.error.mockReset()
})

describe("ThreadView", () => {
  beforeAll(() => import("@/findings/finding-markdown"))

  it("writes no verb beside the person's name", async () => {
    stubFetch({})
    show({ thread: thread(), messages: [message("t1"), message("m2")] })
    expect(await screen.findAllByText("Ada")).not.toHaveLength(0)
    expect(screen.queryByText(/commented|replied/)).toBeNull()
  })

  it("shows one working line with a link to the run while the agent works", async () => {
    stubFetch({})
    show({ thread: thread(), messages: [question] }, (client) =>
      client.setQueryData(
        observeKeys.request(runId),
        request({ status: "running", state: "running" })
      )
    )
    const line = await screen.findByRole("status")
    expect(line).toHaveTextContent(
      "Observe Agent is working on it · View the run"
    )
    expect(screen.getByRole("link", { name: "View the run" })).toHaveAttribute(
      "href",
      `/c/${runId}`
    )
    expect(screen.queryByRole("button", { name: /evidence panel/ })).toBeNull()
  })

  it("answers in the agent's voice and unfolds the evidence from its chip", async () => {
    stubFetch({})
    const { user } = show({
      thread: thread(),
      messages: [question, answered()],
    })
    expect(await screen.findByText("Observe Agent")).toBeVisible()
    expect(screen.queryByText(/is working on it/)).toBeNull()
    expect(screen.getByRole("link", { name: "View the run" })).toBeVisible()
    const chip = await screen.findByRole("button", {
      name: "1 evidence panel",
    })
    expect(screen.queryByRole("article", { name: "Error rate" })).toBeNull()
    await user.click(chip)
    expect(screen.getByRole("article", { name: "Error rate" })).toBeVisible()
  })

  it("links a built-in reply with no answer to its run without fetching the run", async () => {
    const fetch = stubFetch({})
    const failed = { ...reply, body: "The run failed: the run did not finish." }
    show({ thread: thread(), messages: [question, failed] })
    expect(await screen.findByText(failed.body)).toBeVisible()
    expect(screen.getByRole("link", { name: "View the run" })).toHaveAttribute(
      "href",
      `/c/${runId}`
    )
    expect(screen.queryByRole("button", { name: /evidence panel/ })).toBeNull()
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
    expect(
      fetch.mock.calls.some(([input]) =>
        new URL(input.url).pathname.endsWith(`/observe/${runId}`)
      )
    ).toBe(false)
  })

  it("shows a connected agent's answer without a link to its run", async () => {
    stubFetch({})
    show({
      thread: thread(),
      messages: [
        {
          ...question,
          run: { id: runId, status: "succeeded", assigned_kind: "grant" },
        },
        answered({}, false, { ...ada, label: "Ada (via Claude Code)" }),
      ],
    })
    expect(
      await screen.findByRole("button", { name: "1 evidence panel" })
    ).toBeVisible()
    expect(screen.queryByRole("link", { name: "View the run" })).toBeNull()
  })

  it("offers Create observation only to the person who asked", async () => {
    stubFetch({})
    show({
      thread: thread(),
      messages: [question, answered({ observe_worthy: true }, false)],
    })
    expect(
      await screen.findByRole("button", { name: "1 evidence panel" })
    ).toBeVisible()
    expect(
      screen.queryByRole("button", { name: "Create observation" })
    ).toBeNull()
  })

  it.each([
    [
      "an older version",
      { kind: "blocked", reason: pinnedText } as const,
      pinnedText,
    ],
    ["no model", { kind: "no-model" } as const, noModelReviseText],
  ])(
    "holds Revise the dashboard on %s with the composer's reason",
    async (_name, gate, reason) => {
      const fetch = stubFetch({})
      show(
        { thread: thread(), messages: [question, answered()] },
        undefined,
        undefined,
        gate
      )
      const button = await screen.findByRole("button", {
        name: "Revise the dashboard",
      })
      expect(button).toHaveAttribute("aria-disabled", "true")
      expect(button).toHaveAccessibleDescription(reason)
      expect(screen.getByText(reason)).toBeVisible()
      expect(fetch.mock.calls.some(([input]) => input.method === "POST")).toBe(
        false
      )
    }
  )

  it("offers to file a new observation for an answer saved from a dashboard thread", async () => {
    const fetch = stubFetch({
      [`POST /api/v1/observe/${runId}/observation`]: {
        status: 201,
        body: { observation_id: "o-new" },
      },
    })
    const { user } = show({
      thread: thread(),
      messages: [question, answered({ observe_worthy: true })],
    })
    await user.click(
      await screen.findByRole("button", { name: "Create observation" })
    )
    await waitFor(() =>
      expect(
        fetch.mock.calls.map(
          ([sent]) => `${sent.method} ${new URL(sent.url).pathname}`
        )
      ).toContain(`POST /api/v1/observe/${runId}/observation`)
    )
  })

  it("does not offer a new observation for an answer on an observation", async () => {
    show(
      {
        thread: thread({
          target: { kind: "observation", id: "o1" },
          dashboard_id: null,
          version: null,
        }),
        messages: [
          question,
          answered({ dashboard_id: null, observe_worthy: true }),
        ],
      },
      undefined,
      undefined,
      undefined,
      "observation:o1"
    )
    expect(await screen.findByText(/Errors/)).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Create observation" })
    ).toBeNull()
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
  })

  it("applies the suggested change in the thread with Revise the dashboard", async () => {
    const fetch = stubFetch({
      "POST /api/v1/threads/t1/apply": {
        status: 201,
        body: { thread: thread(), message: message("m3"), replayed: false },
      },
    })
    const { user } = show({
      thread: thread(),
      messages: [question, answered()],
    })
    await user.click(
      await screen.findByRole("button", { name: "Revise the dashboard" })
    )
    await waitFor(() =>
      expect(fetch.mock.calls.some(([input]) => input.method === "POST")).toBe(
        true
      )
    )
    const posts = fetch.mock.calls.filter(([input]) => input.method === "POST")
    expect(posts.map(([input]) => new URL(input.url).pathname)).toEqual([
      "/api/v1/threads/t1/apply",
    ])
    expect(await posts[0][0].clone().json()).toEqual({ finding_id: "f1" })
  })

  it.each([
    [
      "a new revision",
      {
        status: 201,
        body: { thread: thread(), message: message("m3"), replayed: false },
      },
    ],
    [
      "a replay of the first one",
      {
        status: 200,
        body: { thread: thread(), message: message("m3"), replayed: true },
      },
    ],
    [
      "a change already applied",
      {
        status: 409,
        body: { error: "already applied", code: "already_applied" },
      },
    ],
  ])("says the revision was asked for after %s", async (_name, answer) => {
    stubFetch({ "POST /api/v1/threads/t1/apply": answer })
    const { user } = show({
      thread: thread(),
      messages: [question, answered()],
    })
    await user.click(
      await screen.findByRole("button", { name: "Revise the dashboard" })
    )
    expect(
      await screen.findByRole("button", { name: "Revision asked for" })
    ).toHaveAttribute("aria-disabled", "true")
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
  })

  it("keeps Revise the dashboard after a refusal", async () => {
    stubFetch({
      "POST /api/v1/threads/t1/apply": {
        status: 429,
        body: { error: "too many revisions", code: "too_many_revisions" },
      },
    })
    const { user } = show({
      thread: thread(),
      messages: [question, answered()],
    })
    const button = await screen.findByRole("button", {
      name: "Revise the dashboard",
    })
    await user.click(button)
    await waitFor(() =>
      expect(button).not.toHaveAttribute("aria-disabled", "true")
    )
    expect(
      screen.queryByRole("button", { name: "Revision asked for" })
    ).toBeNull()
  })

  it("slides in a message that arrives after the thread was drawn, not the ones already there", async () => {
    stubFetch({})
    const first = { thread: thread(), messages: [message("t1"), message("m2")] }
    const { user } = show(
      first,
      undefined,
      message("m3", { body: "Later reply" })
    )
    await screen.findByText("m2")
    await user.click(
      screen.getByRole("button", { name: "Deliver the later message" })
    )
    const arrived = (await screen.findByText("Later reply")).closest("li")
    expect(arrived?.className).toContain("motion-safe:animate-in")
    const earlier = screen.getByText("m2").closest("li")
    expect(earlier?.className).not.toContain("motion-safe:animate-in")
  })

  it("folds a resolved thread to its first line and unfolds it on Show thread", async () => {
    stubFetch({})
    const { user } = show({
      thread: thread({
        status: "resolved",
        allowed_actions: ["reopen"],
        resolved_by_version: 3,
      }),
      messages: [message("t1"), message("m2")],
    })
    expect(await screen.findByText("Resolved by version 3")).toBeVisible()
    expect(screen.getByText("1 reply")).toBeVisible()
    expect(screen.queryByText("m2")).toBeNull()
    await user.click(screen.getByRole("button", { name: "Show thread" }))
    expect(await screen.findByText("m2")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Hide thread" })).toHaveAttribute(
      "aria-expanded",
      "true"
    )
  })

  it("resolves through the thread and keeps the list in step", async () => {
    const fetch = stubFetch({
      "POST /api/v1/threads/t1/resolve": {
        status: 200,
        body: {
          thread: thread({ status: "resolved", allowed_actions: ["reopen"] }),
        },
      },
    })
    const { user, queryClient } = show(
      { thread: thread(), messages: [message("t1")] },
      (client) =>
        client.setQueryData(threadKeys.list(target), {
          pages: [
            {
              threads: [thread()],
              messages: [message("t1")],
              next_cursor: null,
              can_comment: true,
            },
          ],
          pageParams: [undefined],
        })
    )
    await user.click(await screen.findByRole("button", { name: "Resolve" }))
    await waitFor(() =>
      expect(
        fetch.mock.calls.some(([input]) =>
          new URL(input.url).pathname.endsWith("/threads/t1/resolve")
        )
      ).toBe(true)
    )
    await waitFor(() => {
      const data = queryClient.getQueryData<{
        pages: { threads: Thread[] }[]
      }>(threadKeys.list(target))
      expect(data?.pages[0]?.threads[0]?.status).toBe("resolved")
    })
  })
})
