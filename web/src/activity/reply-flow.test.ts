import { describe, expect, it, vi } from "vitest"

import { closeUnlessBusy, dismissLabel, runReply } from "./reply-flow"

function flow(overrides: Partial<Parameters<typeof runReply>[0]> = {}) {
  const state = { open: true }
  const input = {
    state,
    sendToAgent: false,
    postReply: vi.fn().mockResolvedValue(true),
    sendRequest: vi.fn().mockResolvedValue(undefined),
    isOpen: () => state.open,
    onClose: vi.fn(),
    onRequestError: vi.fn(),
    ...overrides,
  }
  return input
}

describe("runReply", () => {
  it("closes after a saved reply", async () => {
    const input = flow()
    await expect(runReply(input)).resolves.toBe("finished")
    expect(input.onClose).toHaveBeenCalledTimes(1)
    expect(input.sendRequest).not.toHaveBeenCalled()
  })

  it("closes after the agent request is sent", async () => {
    const input = flow({ sendToAgent: true })
    await expect(runReply(input)).resolves.toBe("finished")
    expect(input.sendRequest).toHaveBeenCalledTimes(1)
    expect(input.onClose).toHaveBeenCalledTimes(1)
  })

  it("does not close a form that unmounted while the send was in flight", async () => {
    let finish: (saved: boolean) => void = () => {}
    const input = flow({
      sendToAgent: true,
      postReply: vi.fn(
        () =>
          new Promise<boolean>((resolve) => {
            finish = resolve
          })
      ),
    })
    const running = runReply(input)
    input.state.open = false
    finish(true)
    await expect(running).resolves.toBe("finished")
    expect(input.sendRequest).toHaveBeenCalledTimes(1)
    expect(input.onClose).not.toHaveBeenCalled()
  })

  it("asks for a retry and keeps the form open when the reply fails", async () => {
    const input = flow({ postReply: vi.fn().mockResolvedValue(false) })
    await expect(runReply(input)).resolves.toBe("retry")
    expect(input.onClose).not.toHaveBeenCalled()
  })

  it("reports a failed agent request and keeps the form open", async () => {
    const failure = new Error("agent down")
    const input = flow({
      sendToAgent: true,
      sendRequest: vi.fn().mockRejectedValue(failure),
    })
    await expect(runReply(input)).resolves.toBe("retry")
    expect(input.onRequestError).toHaveBeenCalledWith(failure)
    expect(input.onClose).not.toHaveBeenCalled()
  })
})

describe("closeUnlessBusy", () => {
  it("closes when idle", () => {
    const onClose = vi.fn()
    expect(closeUnlessBusy(false, onClose)).toBe(true)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it("does nothing while a send is in flight", () => {
    const onClose = vi.fn()
    expect(closeUnlessBusy(true, onClose)).toBe(false)
    expect(onClose).not.toHaveBeenCalled()
  })
})

describe("dismissLabel", () => {
  it("says Cancel before the reply is saved and Done after", () => {
    expect(dismissLabel(false)).toBe("Cancel")
    expect(dismissLabel(true)).toBe("Done")
  })
})
