import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { AskAgainButton } from "./ask-again"
import { must } from "@/test/must"

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
    const { user, onSent } = render({ chatId: "c1" })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    expect(must(onSent.mock.calls[0])[1]).toBe("same")
    const sent = (await must(fetch.mock.calls[0])[0].json()) as Record<
      string,
      unknown
    >
    expect(sent).toMatchObject({
      prompt: "which logs exist",
      kind: "ask",
      chat_id: "c1",
    })
  })

  it("hands focus on once the question is sent", async () => {
    stubCreate()
    const afterSent = vi.fn()
    const { user, onSent } = render({ chatId: "c1", afterSent })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    await waitFor(() => expect(afterSent).toHaveBeenCalledOnce())
    expect(onSent).toHaveBeenCalledBefore(afterSent)
  })

  it("asks it again about the dashboard the chat is about", async () => {
    const fetch = stubCreate()
    const { user, onSent } = render({
      subject: { kind: "dashboard", id: "d1" },
    })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    await waitFor(() => expect(onSent).toHaveBeenCalledOnce())
    const sent = (await must(fetch.mock.calls[0])[0].json()) as Record<
      string,
      unknown
    >
    expect(sent).toMatchObject({
      prompt: "which logs exist",
      kind: "ask",
      dashboard_id: "d1",
    })
  })

  it("stays off while another run goes on", async () => {
    const fetch = stubCreate()
    const { user } = render({ disabled: true })
    await user.click(await screen.findByRole("button", { name: "Ask again" }))
    expect(fetch).not.toHaveBeenCalled()
  })
})
