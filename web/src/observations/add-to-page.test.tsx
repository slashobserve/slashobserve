import { screen, waitFor } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import type { Message } from "@/comments/thread-queries"
import { renderWithProviders } from "@/test/render"

import { AddToPage, blocksOfMessage } from "./add-to-page"

const mutateAsync = vi.fn()
const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock("sonner", () => ({ toast: toasts }))

vi.mock("./queries", () => ({
  useAddBlock: () => ({ isPending: false, mutateAsync }),
}))

const from = "2026-11-24T10:00:00Z"
const to = "2026-11-24T11:00:00Z"

function message(overrides: Partial<Message>, runId: string | null): Message {
  return {
    id: "m1",
    thread_id: "t1",
    body: "Answered.",
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: runId,
      label: "Ada",
    },
    created_at: from,
    updated_at: from,
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: null,
    reply_to: null,
    ...overrides,
  }
}

function answered(): Message {
  return message(
    {
      answer: {
        can_save: true,
        finding: {
          title: "Checkout is slow",
          markdown: "The 5xx rate rose.",
          evidence: [{ id: "evidence-1" }, { id: "evidence-2" }],
          from,
          to,
        },
      },
    } as unknown as Partial<Message>,
    "r1"
  )
}

beforeEach(() => {
  mutateAsync.mockReset()
  mutateAsync.mockResolvedValue({ block: { id: "b1" } })
  toasts.success.mockReset()
  toasts.error.mockReset()
})

describe("blocksOfMessage", () => {
  it("adds the finding text and each evidence panel for an answer", () => {
    expect(blocksOfMessage(answered())).toEqual([
      {
        kind: "text",
        body: { markdown: "## Checkout is slow\n\nThe 5xx rate rose." },
      },
      { kind: "panel", body: { panel: { id: "evidence-1" }, from, to } },
      { kind: "panel", body: { panel: { id: "evidence-2" }, from, to } },
    ])
  })

  it("adds a person's message as its own text", () => {
    expect(blocksOfMessage(message({ body: "Pool exhausted" }, null))).toEqual([
      { kind: "text", body: { markdown: "Pool exhausted" } },
    ])
  })

  it("offers nothing for an agent reply that carries no answer", () => {
    expect(blocksOfMessage(message({}, "r1"))).toBeNull()
  })
})

describe("AddToPage", () => {
  it("posts the answer's blocks and reports the first one added", async () => {
    const onAdded = vi.fn()
    const { user } = renderWithProviders(
      <AddToPage
        observationId="o1"
        message={answered()}
        canEdit
        onAdded={onAdded}
      />
    )
    await user.click(screen.getByText("Add to page"))
    await waitFor(() => expect(onAdded).toHaveBeenCalledWith("b1"))
    expect(mutateAsync).toHaveBeenCalledTimes(3)
    expect(toasts.success).toHaveBeenCalledWith("Added to the page")
    expect(mutateAsync.mock.calls[0]?.[0]).toEqual({
      kind: "text",
      body: { markdown: "## Checkout is slow\n\nThe 5xx rate rose." },
    })
    expect(onAdded).toHaveBeenCalledWith("b1")
  })

  it("adds the blocks once when the action is clicked twice", async () => {
    let release: () => void = () => {}
    mutateAsync.mockImplementation(
      () =>
        new Promise((resolve) => {
          release = () => resolve({ block: { id: "b1" } })
        })
    )
    const onAdded = vi.fn()
    const { user } = renderWithProviders(
      <AddToPage
        observationId="o1"
        message={answered()}
        canEdit
        onAdded={onAdded}
      />
    )
    await user.click(screen.getByText("Add to page"))
    await user.click(screen.getByText("Add to page"))
    expect(mutateAsync).toHaveBeenCalledTimes(1)
    release()
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(2))
    release()
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(3))
    release()
    await waitFor(() => expect(onAdded).toHaveBeenCalledTimes(1))
    expect(toasts.success).toHaveBeenCalledTimes(1)
  })

  it("says how many blocks were added when one fails and still reports the first", async () => {
    mutateAsync
      .mockResolvedValueOnce({ block: { id: "b1" } })
      .mockResolvedValueOnce({ block: { id: "b2" } })
      .mockRejectedValueOnce(new Error("boom"))
    const onAdded = vi.fn()
    const { user } = renderWithProviders(
      <AddToPage
        observationId="o1"
        message={answered()}
        canEdit
        onAdded={onAdded}
      />
    )
    await user.click(screen.getByText("Add to page"))
    await waitFor(() => expect(toasts.error).toHaveBeenCalledTimes(1))
    expect(toasts.error.mock.calls[0]?.[0]).toMatch(/^Added 2 of 3 blocks: /)
    expect(toasts.success).not.toHaveBeenCalled()
    expect(onAdded).toHaveBeenCalledWith("b1")
  })

  it("renders no action on an agent reply without an answer", () => {
    renderWithProviders(
      <AddToPage
        observationId="o1"
        message={message({}, "r1")}
        canEdit
        onAdded={() => {}}
      />
    )
    expect(screen.queryByText("Add to page")).toBeNull()
  })
})
