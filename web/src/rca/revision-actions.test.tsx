import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { ApiError, problemText } from "@/api/errors"
import { renderWithProviders } from "@/test/render"

import type { RcaSummary } from "./queries"
import { RevisionActions, withdrawProblems } from "./revision-actions"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

const observationId = "0b8e4c55-6c55-4d6a-9d8a-2a8c5b5e9a01"

function respond(status: number, body?: unknown) {
  const requests: { method: string; path: string }[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn((request: Request) => {
      requests.push({
        method: request.method,
        path: new URL(request.url).pathname,
      })
      return Promise.resolve(
        new Response(body === undefined ? null : JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        })
      )
    })
  )
  return requests
}

function revision(allowed: RcaSummary["allowed_actions"]): RcaSummary {
  return {
    revision: 2,
    state: "published",
    allowed_actions: allowed,
  } as Partial<RcaSummary> as RcaSummary
}

function renderActions(canWithdraw: boolean, onWithdrawn = vi.fn()) {
  return {
    onWithdrawn,
    ...renderWithProviders(
      <RevisionActions
        observationId={observationId}
        rca={revision(canWithdraw ? ["withdraw"] : [])}
        canPublish={canWithdraw}
        onWithdrawn={onWithdrawn}
      />,
      { route: { path: "/", url: "/" } }
    ),
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe("RevisionActions", () => {
  it("links to the revision and hides Withdraw from someone who may not edit", async () => {
    renderActions(false)
    const open = await screen.findByRole("link", { name: "Open revision 2" })
    expect(open.getAttribute("href")).toBe(`/o/${observationId}/rca/2`)
    expect(screen.queryByRole("button", { name: "Withdraw" })).toBeNull()
  })

  it("deletes the revision only after the dialog is confirmed", async () => {
    const requests = respond(204)
    const { user, onWithdrawn } = renderActions(true)
    await user.click(await screen.findByRole("button", { name: "Withdraw" }))
    expect(requests).toEqual([])
    await user.click(
      await screen.findByRole("button", { name: "Withdraw revision" })
    )
    await waitFor(() => expect(onWithdrawn).toHaveBeenCalledOnce())
    expect(requests).toEqual([
      {
        method: "DELETE",
        path: `/api/v1/observations/${observationId}/rcas/2`,
      },
    ])
    expect(toasts.success).toHaveBeenCalledWith("Withdrew revision 2")
  })

  it("keeps the revision and says to revoke its links when one is still public", async () => {
    respond(409, {
      error: "revision 2 still has public links",
      code: "rca_shared",
    })
    const { user, onWithdrawn } = renderActions(true)
    await user.click(await screen.findByRole("button", { name: "Withdraw" }))
    await user.click(
      await screen.findByRole("button", { name: "Withdraw revision" })
    )
    await waitFor(() => expect(toasts.error).toHaveBeenCalledOnce())
    expect(onWithdrawn).not.toHaveBeenCalled()
    expect(String(toasts.error.mock.calls[0][0])).toContain(
      "Revoke its public links"
    )
  })
})

describe("withdrawProblemText", () => {
  it("falls back to a retry line for an error that is not the server's", () => {
    expect(problemText(new Error("network"), withdrawProblems)).toBe(
      "The revision could not be withdrawn. Try again."
    )
  })

  it("passes another server refusal through as a sentence", () => {
    expect(
      problemText(
        new ApiError(403, "forbidden_role", "needs edit"),
        withdrawProblems
      )
    ).toBe("Needs edit.")
  })
})
