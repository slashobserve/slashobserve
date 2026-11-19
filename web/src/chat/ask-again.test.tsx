import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AskAgainButton, AskOnButton } from "./ask-again"

function stubCreate() {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async () =>
      new Response(JSON.stringify({ request: { id: "r2", prompt: "q" } }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function render(props: Partial<Parameters<typeof AskAgainButton>[0]> = {}) {
  const onSent = vi.fn()
  const result = renderWithProviders(
    <AskAgainButton
      prompt="which logs exist"
      disabled={false}
      onSent={onSent}
      {...props}
    />,
    { route: { path: "/", url: "/" } }
  )
  return { ...result, onSent }
}

afterEach(() => vi.unstubAllGlobals())

describe("AskAgainButton", () => {
  it("sends the same question as a follow-up of the chat and appends it", async () => {
    const fetch = stubCreate()
    const { user, onSent } = render({ followsId: "r1" })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    expect(onSent.mock.calls[0][1]).toBe("same")
    const sent = (await fetch.mock.calls[0][0].json()) as Record<
      string,
      unknown
    >
    expect(sent).toMatchObject({
      prompt: "which logs exist",
      follows_id: "r1",
    })
  })

  it("hands focus on once the question is sent", async () => {
    stubCreate()
    const afterSent = vi.fn()
    const { user, onSent } = render({ followsId: "r1", afterSent })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    await waitFor(() => expect(afterSent).toHaveBeenCalledOnce())
    expect(onSent).toHaveBeenCalledBefore(afterSent)
  })

  it("sends it into the dashboard the chat is about", async () => {
    const fetch = stubCreate()
    const { user, onSent } = render({ dashboardId: "d1" })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    const sent = (await fetch.mock.calls[0][0].json()) as Record<
      string,
      unknown
    >
    expect(sent).toMatchObject({
      prompt: "which logs exist",
      dashboard_id: "d1",
    })
    expect(sent).not.toHaveProperty("follows_id")
  })

  it("stays off while another run goes on", async () => {
    const fetch = stubCreate()
    const { user } = render({ disabled: true })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    expect(fetch).not.toHaveBeenCalled()
  })
})

describe("AskOnButton", () => {
  it("revises the named dashboard with the same prompt and follows the run", async () => {
    const fetch = stubCreate()
    const onSent = vi.fn()
    const { user } = renderWithProviders(
      <AskOnButton
        askOn={{ dashboardId: "d1", title: "Checkout" }}
        prompt="add a latency row"
        disabled={false}
        onSent={onSent}
      />,
      { route: { path: "/", url: "/" } }
    )
    await user.click(
      await screen.findByRole("button", { name: "Ask this on Checkout" })
    )
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    const sent = (await fetch.mock.calls[0][0].json()) as Record<
      string,
      unknown
    >
    expect(sent).toMatchObject({
      prompt: "add a latency row",
      dashboard_id: "d1",
    })
    expect(onSent.mock.calls[0][1]).toBe("chat")
  })
})
