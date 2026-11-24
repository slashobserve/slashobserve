import { fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"

import { dashboardKeys, folderKeys, observeKeys, threadKeys } from "@/api/keys"
import { ApiError } from "@/api/errors"
import type { Message, Thread } from "@/comments/thread-queries"
import type { Finding } from "@/findings/queries"
import { starterNote } from "@/dashboards/permissions"
import { renderWithProviders, testQueryClient } from "@/test/render"

import {
  AnswerCard,
  evidenceChipText,
  reviseFailureText,
  starterCardNote,
  threadLabel,
} from "./answer-card"

vi.mock("@/findings/evidence-panel", () => ({
  EvidencePanel: ({ panel }: { panel: { title: string } }) => (
    <article aria-label={panel.title} />
  ),
}))

vi.mock("@/dashboards/permissions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/dashboards/permissions")>()),
  useLatestProvisioned: () => false,
  useLatestPermissions: () => ({
    comment: true,
    edit: true,
    manage: false,
    create: false,
  }),
}))

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

const text = "The error rate **doubled** at 10:02."

function finding(patch: Partial<Finding> = {}): Finding {
  return {
    id: "f1",
    observation_id: null,
    agent_request_id: "r1",
    dashboard_id: "d1",
    folder_id: null,
    ask_on: null,
    restricted: false,
    title: "Checkout errors rose after the deploy",
    markdown: text,
    evidence: [
      { id: "e1", type: "timeseries", title: "Error rate" },
      { id: "e2", type: "timeseries", title: "Latency" },
    ] as unknown as Finding["evidence"],
    from: "2026-11-06T10:00:00Z",
    to: "2026-11-06T11:00:00Z",
    as_of: null,
    annotation_id: null,
    created_by: { kind: "user", id: "u1", name: "Ada" },
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: "r1",
      label: "Observe Agent for Ada",
    },
    created_at: "2026-11-06T11:05:00Z",
    url: null,
    suggested_change: null,
    observe_worthy: false,
    ...patch,
  }
}

function thread(id: string, status: Thread["status"]): Thread {
  return {
    id,
    target: { kind: "dashboard", id: "d1" },
    dashboard_id: "d1",
    version: 1,
    panel_id: null,
    panel_present: true,
    status,
    allowed_actions: ["reply"],
    unread: false,
    created_at: "2026-11-06T09:00:00Z",
    updated_at: "2026-11-06T09:00:00Z",
    resolved_at: null,
    resolved_actor: null,
    resolved_by_version: null,
    revision: null,
    anchor: null,
    block_id: null,
    agent: null,
    agent_off: false,
  }
}

function rootMessage(threadId: string, body: string): Message {
  return {
    id: threadId,
    thread_id: threadId,
    body,
    actor: finding().actor,
    created_at: "2026-11-06T09:00:00Z",
    updated_at: "2026-11-06T09:00:00Z",
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: null,
    reply_to: null,
  }
}

function stubFetch(body: unknown, status = 200) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async () =>
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

async function sent(fetch: ReturnType<typeof stubFetch>, call = 0) {
  const request = fetch.mock.calls[call][0]
  const raw = await request.clone().text()
  return {
    line: `${request.method} ${new URL(request.url).pathname}`,
    body: raw === "" ? null : (JSON.parse(raw) as unknown),
  }
}

type LatestDashboard = {
  permissions: { comment: boolean; edit: boolean }
  provisioned: boolean
}

function renderCard(
  props: Partial<Parameters<typeof AnswerCard>[0]> = {},
  latest: LatestDashboard = {
    permissions: { comment: true, edit: true },
    provisioned: false,
  }
) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(dashboardKeys.latest("d1"), {
    dashboard: {
      id: "d1",
      title: "Audit overview",
      latest_version: 4,
      folder: { id: "audit", title: "Audit" },
      ...latest,
    },
  })
  queryClient.setQueryData(folderKeys.list(), [
    {
      id: "mine",
      title: "My Dashboards",
      general: false,
      personal: true,
      permissions: { create: true },
    },
  ])
  queryClient.setQueryData(threadKeys.list("dashboard:d1"), {
    pages: [
      {
        threads: [thread("t1", "open"), thread("t2", "resolved")],
        messages: [
          rootMessage("t1", "Why did checkout errors rise?"),
          rootMessage("t2", "Old question"),
        ],
        next_cursor: null,
        can_comment: true,
      },
    ],
    pageParams: [undefined],
  })
  queryClient.setQueryData(observeKeys.request("r1"), {})
  return renderWithProviders(
    <AnswerCard
      finding={finding()}
      requestId="r1"
      canSave={false}
      {...props}
    />,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

function requestStale(queryClient: ReturnType<typeof testQueryClient>) {
  return queryClient.getQueryState(observeKeys.request("r1"))?.isInvalidated
}

afterEach(() => {
  vi.unstubAllGlobals()
  toasts.success.mockReset()
  toasts.error.mockReset()
})

describe("evidenceChipText", () => {
  it("counts the evidence panels", () => {
    expect(evidenceChipText(1)).toBe("1 evidence panel")
    expect(evidenceChipText(2)).toBe("2 evidence panels")
  })
})

describe("threadLabel", () => {
  it("names a thread by its text on one line, cut at 60 characters", () => {
    expect(threadLabel("Why did checkout\n errors rise?")).toBe(
      "Why did checkout errors rise?"
    )
    const label = threadLabel("a".repeat(80))
    expect([...label]).toHaveLength(60)
    expect(label.endsWith("…")).toBe(true)
  })
})

describe("reviseFailureText", () => {
  it("names a starter dashboard when the server refuses the change as provisioned", () => {
    expect(
      reviseFailureText(
        new ApiError(409, "provisioned", "dashboard is provisioned")
      )
    ).toBe(starterNote)
  })

  it.each([
    [
      new ApiError(429, "too_many_revisions", "too many revisions"),
      "Too many revisions.",
    ],
    [new Error(""), "The change could not be asked for. Try again."],
  ])("words any other refusal from its message", (error, text) => {
    expect(reviseFailureText(error)).toBe(text)
  })
})

describe("AnswerCard", () => {
  beforeAll(() => import("@/findings/finding-markdown"))

  it("keeps the evidence folded until its chip unfolds it in place", async () => {
    const { user } = renderCard()
    expect(await screen.findByText("doubled")).toBeInTheDocument()
    const chip = screen.getByRole("button", { name: "2 evidence panels" })
    expect(chip).toHaveAttribute("aria-expanded", "false")
    expect(screen.queryByRole("article", { name: "Error rate" })).toBeNull()
    await user.click(chip)
    expect(chip).toHaveAttribute("aria-expanded", "true")
    expect(screen.getByRole("article", { name: "Error rate" })).toBeVisible()
    expect(screen.getByRole("article", { name: "Latency" })).toBeVisible()
    await user.click(chip)
    expect(screen.queryByRole("article", { name: "Error rate" })).toBeNull()
  })

  it("shows no chip for an answer without evidence", async () => {
    renderCard({ finding: finding({ evidence: [] }) })
    expect(await screen.findByText("doubled")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /evidence panel/ })).toBeNull()
  })

  it("applies the suggested change with Revise the dashboard", async () => {
    const onRevise = vi.fn(async () => false)
    const { user } = renderCard({
      finding: finding({ suggested_change: "Add a p99 latency panel" }),
      revise: { onRevise, pending: false, blocked: null, dashboardId: "d1" },
    })
    expect(
      await screen.findByText("Suggested change: Add a p99 latency panel")
    ).toBeVisible()
    await user.click(
      screen.getByRole("button", { name: "Revise the dashboard" })
    )
    expect(onRevise).toHaveBeenCalledOnce()
  })

  it("keeps focus on the button and announces it once the revision is asked for", async () => {
    const { user } = renderCard({
      finding: finding({ suggested_change: "Add a p99 latency panel" }),
      revise: {
        onRevise: vi.fn(async () => true),
        pending: false,
        blocked: null,
        dashboardId: "d1",
      },
    })
    const button = await screen.findByRole("button", {
      name: "Revise the dashboard",
    })
    button.focus()
    await user.keyboard("{Enter}")
    expect(
      await screen.findByRole("button", { name: "Revision asked for" })
    ).toHaveAttribute("aria-disabled", "true")
    expect(document.activeElement).toBe(button)
    expect(screen.getByRole("status")).toHaveTextContent(
      "The revision was asked for."
    )
  })

  it("offers Revise the dashboard once the latest dashboard says the reader may change it", async () => {
    renderCard(
      {
        finding: finding({ suggested_change: "Add a p99 latency panel" }),
        revise: {
          onRevise: vi.fn(async () => false),
          pending: false,
          blocked: null,
          dashboardId: "d1",
        },
      },
      { permissions: { comment: true, edit: false }, provisioned: false }
    )
    expect(
      await screen.findByRole("button", { name: "Revise the dashboard" })
    ).toBeVisible()
    expect(screen.queryByText(starterCardNote)).toBeNull()
  })

  it.each([
    ["a starter dashboard", { comment: true, edit: true }, true, true],
    [
      "a dashboard the reader can no longer comment on",
      { comment: false, edit: false },
      false,
      false,
    ],
  ])(
    "keeps the suggested change but offers no Revise on %s",
    async (_name, permissions, provisioned, noted) => {
      renderCard(
        {
          finding: finding({ suggested_change: "Add a p99 latency panel" }),
          revise: {
            onRevise: vi.fn(async () => false),
            pending: false,
            blocked: null,
            dashboardId: "d1",
          },
        },
        { permissions, provisioned }
      )
      expect(
        await screen.findByText("Suggested change: Add a p99 latency panel")
      ).toBeVisible()
      expect(
        screen.queryByRole("button", { name: "Revise the dashboard" })
      ).toBeNull()
      expect(screen.queryByText(starterCardNote) !== null).toBe(noted)
      expect(screen.queryByRole("button", { name: "Duplicate" }) !== null).toBe(
        noted
      )
    }
  )

  it("duplicates a starter from its answer's suggested change", async () => {
    const { user } = renderCard(
      {
        finding: finding({ suggested_change: "Add a p99 latency panel" }),
        revise: {
          onRevise: vi.fn(async () => false),
          pending: false,
          blocked: null,
          dashboardId: "d1",
        },
      },
      { permissions: { comment: true, edit: false }, provisioned: true }
    )
    await user.click(await screen.findByRole("button", { name: "Duplicate" }))
    const dialog = await screen.findByRole("dialog", {
      name: "Duplicate dashboard",
    })
    expect(dialog).toHaveTextContent('Saves version 4 of "Audit overview"')
    expect(within(dialog).getByRole("combobox")).toHaveTextContent(
      "My Dashboards"
    )
  })

  it("shows a suggested change to a reader who cannot revise, without the button", async () => {
    const view = renderCard({
      revise: {
        onRevise: vi.fn(async () => false),
        pending: false,
        blocked: null,
        dashboardId: "d1",
      },
    })
    await screen.findByText("doubled")
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
    view.unmount()
    renderCard({ finding: finding({ suggested_change: "Add a panel" }) })
    await screen.findByText("doubled")
    expect(screen.getByText("Suggested change: Add a panel")).toBeVisible()
    expect(
      screen.queryByRole("button", { name: "Revise the dashboard" })
    ).toBeNull()
  })

  it("shows Create observation in the action row for an answer its asker may save, worthy or not", async () => {
    const fetch = stubFetch(
      { observation_id: "o5", url: "/o/o5", replayed: false },
      201
    )
    const { user } = renderCard({
      finding: finding({ observe_worthy: true }),
      canSave: true,
    })
    await user.click(
      await screen.findByRole("button", { name: "Create observation" })
    )
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1))
    expect((await sent(fetch)).line).toBe("POST /api/v1/observe/r1/observation")
  })

  it("shows Create observation in the action row of an answer the model did not flag", async () => {
    renderCard({
      finding: finding({ observe_worthy: false }),
      canSave: true,
    })
    await screen.findByText("doubled")
    expect(
      screen.getByRole("button", { name: "Create observation" })
    ).toBeVisible()
  })

  it("shows Create observation once, moving it under the evidence while that is open", async () => {
    const { user } = renderCard({ canSave: true })
    await screen.findByText("doubled")
    expect(
      screen.getAllByRole("button", { name: "Create observation" })
    ).toHaveLength(1)
    await user.click(screen.getByRole("button", { name: "2 evidence panels" }))
    expect(
      screen.getAllByRole("button", { name: "Create observation" })
    ).toHaveLength(1)
  })

  it("shows Create observation on an answer with no evidence panels", async () => {
    renderCard({ finding: finding({ evidence: [] }), canSave: true })
    await screen.findByText("doubled")
    expect(
      screen.getByRole("button", { name: "Create observation" })
    ).toBeVisible()
  })

  it("offers no Create observation to a reader who may not save the answer", async () => {
    renderCard({ finding: finding({ observe_worthy: true }), canSave: false })
    await screen.findByText("doubled")
    expect(
      screen.queryByRole("button", { name: "Create observation" })
    ).toBeNull()
  })

  it("links to the observation an answer was already saved in", async () => {
    const { user } = renderCard({
      finding: finding({ observation_id: "o9" }),
      canSave: true,
    })
    await user.click(
      await screen.findByRole("button", { name: "2 evidence panels" })
    )
    expect(
      screen.getByRole("link", { name: "Open the observation" })
    ).toHaveAttribute("href", "/o/o9")
    expect(
      screen.queryByRole("button", { name: "Create observation" })
    ).toBeNull()
  })

  it("offers Pin and Make a dashboard only once the answer is in an observation", async () => {
    const view = renderCard({ finding: finding({ dashboard_id: "d1" }) })
    await screen.findByText("doubled")
    expect(
      screen.queryByRole("button", { name: "Make a dashboard" })
    ).toBeNull()
    expect(
      screen.queryByRole("button", { name: "Pin to dashboard" })
    ).toBeNull()
    view.unmount()
    renderCard({
      finding: finding({ dashboard_id: "d1", observation_id: "o9" }),
    })
    expect(
      await screen.findByRole("button", { name: "Make a dashboard" })
    ).toBeVisible()
    expect(
      screen.getByRole("button", { name: "Pin to dashboard" })
    ).toBeVisible()
    expect(
      screen.getByRole("button", { name: "2 evidence panels" })
    ).toBeVisible()
  })

  it("hands the chat to a dashboard made from the answer", async () => {
    const dashboardId = "33333333-3333-4333-8333-333333333333"
    stubFetch({ dashboard: { id: dashboardId, title: "Checkout errors" } }, 201)
    const { user, router } = renderCard({
      finding: finding({ observation_id: "o9" }),
      chatId: "c1",
    })
    await user.click(
      await screen.findByRole("button", { name: "Make a dashboard" })
    )
    await vi.waitFor(() =>
      expect(router?.state.location.search).toEqual({ tab: "chat", chat: "c1" })
    )
  })

  it("links to the observation in its row when there is no evidence to unfold", async () => {
    renderCard({ finding: finding({ evidence: [], observation_id: "o9" }) })
    expect(
      await screen.findByRole("link", { name: "Open the observation" })
    ).toHaveAttribute("href", "/o/o9")
  })

  it("has no menu unless one is asked for", async () => {
    renderCard()
    await screen.findByText("doubled")
    expect(screen.queryByRole("button", { name: "Answer actions" })).toBeNull()
  })

  it("shows the answer to everyone from the asker's menu", async () => {
    const fetch = stubFetch({
      thread_id: "t9",
      message_id: "m9",
      replayed: false,
    })
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: /^Show to everyone/ })
    )
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(await sent(fetch)).toEqual({
      line: "POST /api/v1/observe/r1/share",
      body: {},
    })
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith(
        "Everyone here can see this answer now"
      )
    )
  })

  it("offers only Share to thread for an answer that stays private", async () => {
    stubFetch({})
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        showToEveryone: false,
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    expect(
      await screen.findByRole("menuitem", { name: /^Share to thread/ })
    ).toBeVisible()
    expect(
      screen.queryByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeNull()
  })

  it("shares the answer to an open thread picked from the menu", async () => {
    const fetch = stubFetch({
      thread_id: "t1",
      message_id: "m9",
      replayed: false,
    })
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: /^Share to thread/ })
    )
    const choice = await screen.findByRole("menuitem", {
      name: "Why did checkout errors rise?",
    })
    expect(screen.queryByRole("menuitem", { name: "Old question" })).toBeNull()
    fireEvent.click(choice)
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(await sent(fetch)).toEqual({
      line: "POST /api/v1/observe/r1/share",
      body: { thread_id: "t1" },
    })
  })

  it("says the answer is already shared in its thread when the share replays", async () => {
    stubFetch({ thread_id: "t5", message_id: "m5", replayed: true })
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: /^Share to thread/ })
    )
    fireEvent.click(
      await screen.findByRole("menuitem", {
        name: "Why did checkout errors rise?",
      })
    )
    await waitFor(() =>
      expect(toasts.success).toHaveBeenCalledWith(
        "This answer is already shared in its thread"
      )
    )
  })

  async function shareToEveryone(user: ReturnType<typeof renderCard>["user"]) {
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    await user.click(
      await screen.findByRole("menuitem", { name: /^Show to everyone/ })
    )
    await waitFor(() => expect(toasts.success).toHaveBeenCalled())
    await user.click(screen.getByRole("button", { name: "Answer actions" }))
  }

  function stubResponses(...responses: { status: number; body?: unknown }[]) {
    const queue = [...responses]
    const fetch = vi.fn<(request: Request) => Promise<Response>>(async () => {
      const next = queue.shift() ?? { status: 500 }
      return next.body === undefined
        ? new Response(null, { status: next.status })
        : new Response(JSON.stringify(next.body), {
            status: next.status,
            headers: { "Content-Type": "application/json" },
          })
    })
    vi.stubGlobal("fetch", fetch)
    return fetch
  }

  const shared = {
    status: 201,
    body: { thread_id: "t9", message_id: "m9", replayed: false },
  }

  it("links to the thread a shared answer is in and offers Withdraw", async () => {
    stubResponses(shared)
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await shareToEveryone(user)
    expect(
      await screen.findByRole("menuitem", { name: /^Shared in the thread/ })
    ).toHaveAttribute("href", "/d/d1?thread=t9")
    expect(screen.getByRole("menuitem", { name: /^Withdraw/ })).toBeVisible()
    expect(
      screen.queryByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeNull()
    expect(
      screen.queryByRole("menuitem", { name: /^Share to thread/ })
    ).toBeNull()
  })

  it("knows an answer the request says is already shared", async () => {
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: "t5",
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    expect(
      await screen.findByRole("menuitem", { name: /^Shared in the thread/ })
    ).toHaveAttribute("href", "/d/d1?thread=t5")
    expect(
      screen.queryByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeNull()
  })

  it("keeps Shared in the thread and Withdraw on an answer shared and then saved", async () => {
    const { user } = renderCard({
      finding: finding({ observation_id: "o9" }),
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: "t5",
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    expect(
      await screen.findByRole("menuitem", { name: /^Shared in the thread/ })
    ).toHaveAttribute("href", "/d/d1?thread=t5")
    expect(screen.getByRole("menuitem", { name: /^Withdraw/ })).toBeVisible()
  })

  it("offers no share choices on a saved answer that is not shared", async () => {
    const { user } = renderCard({
      finding: finding({ observation_id: "o9" }),
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    expect(
      await screen.findByRole("menuitem", { name: /^Copy answer/ })
    ).toBeVisible()
    expect(
      screen.queryByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeNull()
    expect(
      screen.queryByRole("menuitem", { name: /^Share to thread/ })
    ).toBeNull()
  })

  it("withdraws a shared answer back to only the asker", async () => {
    const fetch = stubResponses(shared, { status: 204 })
    const { user, queryClient } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await shareToEveryone(user)
    await waitFor(() => expect(requestStale(queryClient)).toBe(true))
    queryClient.setQueryData(observeKeys.request("r1"), {})
    expect(
      await screen.findByRole("menuitem", {
        name: "Withdraw Removes your question and this answer from the thread, and the thread too when sharing opened it, while no one else has written in it.",
      })
    ).toBeVisible()
    await user.click(await screen.findByRole("menuitem", { name: /^Withdraw/ }))
    await waitFor(() => expect(requestStale(queryClient)).toBe(true))
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
    expect(await sent(fetch, 1)).toEqual({
      line: "DELETE /api/v1/threads/t9/share",
      body: null,
    })
    await waitFor(() =>
      expect(toasts.success).toHaveBeenLastCalledWith(
        "Only you and Admins can see this answer again"
      )
    )
    await user.click(screen.getByRole("button", { name: "Answer actions" }))
    expect(
      await screen.findByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeVisible()
  })

  it("keeps the answer shared once someone else wrote in the thread", async () => {
    stubResponses(shared, {
      status: 409,
      body: { error: "someone else wrote", code: "others_wrote" },
    })
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await shareToEveryone(user)
    await user.click(await screen.findByRole("menuitem", { name: /^Withdraw/ }))
    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith(
        "Someone else wrote in the thread, so the answer stays shared."
      )
    )
    await user.click(screen.getByRole("button", { name: "Answer actions" }))
    expect(
      await screen.findByRole("menuitem", { name: /^Shared in the thread/ })
    ).toBeVisible()
  })

  it("forgets a share the server no longer has", async () => {
    stubResponses(shared, {
      status: 404,
      body: { error: "not found", code: "not_found" },
    })
    const { user } = renderCard({
      menu: {
        shareTarget: "dashboard:d1",
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await shareToEveryone(user)
    await user.click(await screen.findByRole("menuitem", { name: /^Withdraw/ }))
    await waitFor(() =>
      expect(toasts.error).toHaveBeenCalledWith(
        "This answer is no longer shared in a thread."
      )
    )
    await user.click(screen.getByRole("button", { name: "Answer actions" }))
    expect(
      await screen.findByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeVisible()
  })

  it("offers only Copy answer when there is nowhere to share", async () => {
    const { user } = renderCard({
      menu: {
        shareTarget: null,
        sharedThreadId: null,
        subjectObservationId: null,
      },
    })
    await user.click(
      await screen.findByRole("button", { name: "Answer actions" })
    )
    const writeText = vi.spyOn(navigator.clipboard, "writeText")
    await user.click(
      await screen.findByRole("menuitem", { name: /^Copy answer/ })
    )
    expect(
      screen.queryByRole("menuitem", { name: /^Show to everyone/ })
    ).toBeNull()
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(text))
  })
})
