import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { ApiError } from "@/api/errors"
import { renderWithProviders, testQueryClient } from "@/test/render"

import type { ShownLevels } from "./levels"
import { VolumeStrip } from "./volume-strip"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

const range = { from: "now-1h", to: "now" }
const query = "{app=1}"

const volume = {
  total: 20,
  step_ms: 60000,
  partial: false,
  buckets: [
    {
      start: "2026-11-06T10:00:00Z",
      error: 8,
      warning: 2,
      info: 10,
      other: 0,
    },
  ],
}

function setup({
  shown,
  timedOut = false,
}: { shown?: ShownLevels; timedOut?: boolean } = {}) {
  const pick = vi.fn()
  const clear = vi.fn()
  const workspace = {
    update: vi.fn(),
    results: {
      isSuccess: !timedOut,
      isError: timedOut,
      isPlaceholderData: false,
      error: timedOut
        ? new ApiError(504, "query_timeout", "Query timed out after 15s")
        : null,
    },
    levels: { shown, hidden: new Set(), pick, clear },
  } as unknown as LogsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(
    ["explore", "ds", "volume", { query, ...range }],
    volume
  )
  renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <VolumeStrip uid="ds" query={query} {...range} />
    </LogsWorkspaceContext>,
    { queryClient }
  )
  return { pick, clear }
}

describe("VolumeStrip level pills", () => {
  it("picks one level on a plain click", async () => {
    const { pick } = setup()
    fireEvent.click(await screen.findByRole("button", { name: /^Error/ }))
    expect(pick).toHaveBeenCalledWith("error", false)
  })

  it.each(["shiftKey", "metaKey", "ctrlKey"])(
    "adds or removes a level with %s",
    async (modifier) => {
      const { pick } = setup()
      fireEvent.click(await screen.findByRole("button", { name: /^Info/ }), {
        [modifier]: true,
      })
      expect(pick).toHaveBeenCalledWith("info", true)
    }
  )

  it("marks all as selected when no level is picked", async () => {
    setup()
    expect(await screen.findByRole("button", { name: /^All/ })).toHaveAttribute(
      "aria-pressed",
      "true"
    )
    expect(screen.getByRole("button", { name: /^Error/ })).toHaveAttribute(
      "aria-pressed",
      "false"
    )
  })

  it("marks the picked level and dims the others", async () => {
    setup({ shown: ["error"] })
    const error = await screen.findByRole("button", { name: /^Error/ })
    expect(error).toHaveAttribute("aria-pressed", "true")
    expect(error).not.toHaveAttribute("data-dimmed")
    const info = screen.getByRole("button", { name: /^Info/ })
    expect(info).toHaveAttribute("aria-pressed", "false")
    expect(info).toHaveAttribute("data-dimmed")
    expect(screen.getByRole("button", { name: /^All/ })).toHaveAttribute(
      "aria-pressed",
      "false"
    )
  })

  it("clears the pick from All", async () => {
    const { clear } = setup({ shown: ["error"] })
    fireEvent.click(await screen.findByRole("button", { name: /^All/ }))
    expect(clear).toHaveBeenCalled()
  })
})

describe("VolumeStrip after a failed query", () => {
  it("points to the results card instead of waiting or offering a second Retry", async () => {
    setup({ timedOut: true })
    expect(
      await screen.findByText("The query failed, see below.")
    ).toBeVisible()
    expect(screen.queryByText(/when the query answers/)).toBeNull()
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull()
  })
})
