import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { DeleteDashboardDialog } from "./delete-dashboard-dialog"

function deferredFetch() {
  let respond: (response: Response) => void = () => undefined
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    () =>
      new Promise((resolve) => {
        respond = resolve
      })
  )
  vi.stubGlobal("fetch", fetch)
  return { fetch, respond: (response: Response) => respond(response) }
}

function renderDialog() {
  const onOpenChange = vi.fn()
  const onDeleted = vi.fn()
  const result = renderWithProviders(
    <DeleteDashboardDialog
      dashboard={{ id: "d1", title: "Checkout", versions: 3 }}
      open
      onOpenChange={onOpenChange}
      onDeleted={onDeleted}
      finalFocus={() => true}
    />
  )
  return { ...result, onOpenChange, onDeleted }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("DeleteDashboardDialog", () => {
  it("closes through the open handler on cancel without deleting", async () => {
    const { fetch } = deferredFetch()
    const { user, onOpenChange, onDeleted } = renderDialog()
    await user.click(await screen.findByRole("button", { name: "Cancel" }))
    expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
    expect(onDeleted).not.toHaveBeenCalled()
    expect(fetch).not.toHaveBeenCalled()
  })

  it("disables confirm while the delete is pending, then reports it deleted", async () => {
    const { fetch, respond } = deferredFetch()
    const { user, onDeleted } = renderDialog()
    const confirm = await screen.findByRole("button", {
      name: "Delete dashboard",
    })
    expect(confirm).not.toHaveAttribute("aria-disabled", "true")

    await user.click(confirm)
    await waitFor(() =>
      expect(confirm).toHaveAttribute("aria-disabled", "true")
    )
    expect(confirm).toHaveAttribute("aria-busy", "true")
    await user.click(confirm)
    expect(fetch).toHaveBeenCalledOnce()
    const request = fetch.mock.calls[0][0]
    expect(request.method).toBe("DELETE")
    expect(new URL(request.url).pathname).toBe("/api/v1/dashboards/d1")
    expect(onDeleted).not.toHaveBeenCalled()

    respond(new Response(null, { status: 204 }))
    await waitFor(() =>
      expect(onDeleted).toHaveBeenCalledExactlyOnceWith(false)
    )
  })
})
