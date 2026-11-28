import { describe, expect, it, vi } from "vitest"

import { connect, protocolVersion, type HostHandlers } from "./host"
import { must } from "@/test/must"

function fakeParent() {
  const sent: Record<string, unknown>[] = []
  const parent = {
    postMessage: (message: Record<string, unknown>) => sent.push(message),
  } as unknown as Window
  const deliver = (data: Record<string, unknown>) =>
    window.dispatchEvent(
      new MessageEvent("message", {
        data: { jsonrpc: "2.0", ...data },
        source: parent as MessageEventSource,
      })
    )
  return { parent, sent, deliver }
}

function handlers(): HostHandlers {
  return {
    onToolResult: vi.fn(),
    onToolCancelled: vi.fn(),
    onContext: vi.fn(),
  }
}

async function connected(h = handlers()) {
  const fake = fakeParent()
  const pending = connect(fake.parent, window, h)
  const init = must(fake.sent[0])
  fake.deliver({
    id: init.id as number,
    result: {
      protocolVersion,
      hostInfo: { name: "host", version: "1" },
      hostContext: {
        theme: "dark",
        containerDimensions: { height: 300 },
      },
    },
  })
  const host = await pending
  return { ...fake, host, handlers: h }
}

describe("connect", () => {
  it("opens with ui/initialize, then tells the host it is initialized", async () => {
    const { sent, handlers: h, host } = await connected()
    expect(sent[0]).toMatchObject({
      method: "ui/initialize",
      params: { protocolVersion: "2026-01-26" },
    })
    expect(sent[1]).toMatchObject({ method: "ui/notifications/initialized" })
    expect(sent[1]).not.toHaveProperty("id")
    expect(h.onContext).toHaveBeenCalledWith({
      theme: "dark",
      containerDimensions: { height: 300 },
    })
    host.disconnect()
  })

  it("hands over a tool result and a cancellation", async () => {
    const { deliver, handlers: h, host } = await connected()
    const result = { structuredContent: { title: "x" } }
    deliver({ method: "ui/notifications/tool-result", params: result })
    deliver({ method: "ui/notifications/tool-cancelled", params: {} })
    expect(h.onToolResult).toHaveBeenCalledWith(result)
    expect(h.onToolCancelled).toHaveBeenCalled()
    host.disconnect()
  })

  it("merges a changed host context", async () => {
    const { deliver, handlers: h, host } = await connected()
    deliver({
      method: "ui/notifications/host-context-changed",
      params: { theme: "light" },
    })
    expect(h.onContext).toHaveBeenLastCalledWith({
      theme: "light",
      containerDimensions: { height: 300 },
    })
    expect(host.context().theme).toBe("light")
    host.disconnect()
  })

  it("answers ping and teardown, and refuses any other request", async () => {
    const { deliver, sent, host } = await connected()
    deliver({ id: 7, method: "ping" })
    deliver({ id: 8, method: "ui/resource-teardown", params: {} })
    deliver({ id: 9, method: "tools/call" })
    expect(sent.slice(2)).toEqual([
      { jsonrpc: "2.0", id: 7, result: {} },
      { jsonrpc: "2.0", id: 8, result: {} },
      {
        jsonrpc: "2.0",
        id: 9,
        error: { code: -32601, message: "method not found" },
      },
    ])
    host.disconnect()
  })

  it("asks the host to open a link and to take a new size", async () => {
    const { deliver, sent, host } = await connected()
    const opened = host.openLink("https://observe.test/d/1")
    const request = must(sent[sent.length - 1])
    expect(request).toMatchObject({
      method: "ui/open-link",
      params: { url: "https://observe.test/d/1" },
    })
    deliver({ id: request.id as number, result: {} })
    await expect(opened).resolves.toBeUndefined()
    host.reportSize(500, 320)
    expect(sent[sent.length - 1]).toMatchObject({
      method: "ui/notifications/size-changed",
      params: { width: 500, height: 320 },
    })
    host.disconnect()
  })

  it("rejects a refused request", async () => {
    const { deliver, sent, host } = await connected()
    const opened = host.openLink("https://observe.test/d/1")
    deliver({
      id: must(sent[sent.length - 1]).id as number,
      error: { code: -32000, message: "denied" },
    })
    await expect(opened).rejects.toThrow("denied")
    host.disconnect()
  })

  it("ignores messages that are not from the host", async () => {
    const { handlers: h, host } = await connected()
    window.dispatchEvent(
      new MessageEvent("message", {
        data: { jsonrpc: "2.0", method: "ui/notifications/tool-result" },
        source: window as MessageEventSource,
      })
    )
    expect(h.onToolResult).not.toHaveBeenCalled()
    host.disconnect()
  })
})
