import { act, render, screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import recorded from "./fixtures/show-panel-result.json"
import { chartHeightFor, McpPanel, viewOf } from "./mcp-panel"
import { payloadOf } from "./payload"

const network = {
  fetch: vi.fn(),
  xhr: vi.fn(),
  socket: vi.fn(),
  source: vi.fn(),
  beacon: vi.fn(),
}

beforeEach(() => {
  vi.stubGlobal("fetch", network.fetch)
  vi.stubGlobal("XMLHttpRequest", network.xhr)
  vi.stubGlobal("WebSocket", network.socket)
  vi.stubGlobal("EventSource", network.source)
  Object.defineProperty(navigator, "sendBeacon", {
    configurable: true,
    value: network.beacon,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  Object.values(network).forEach((spy) => spy.mockClear())
})

const asStat = {
  ...recorded,
  structuredContent: {
    ...recorded.structuredContent,
    panel: { ...recorded.structuredContent.panel, type: "stat" },
  },
}

function host() {
  const sent: Record<string, unknown>[] = []
  const parent = {
    postMessage: (message: Record<string, unknown>) => sent.push(message),
  } as unknown as Window
  const deliver = (data: Record<string, unknown>) =>
    act(() => {
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { jsonrpc: "2.0", ...data },
          source: parent as MessageEventSource,
        })
      )
    })
  const handshake = async (theme: "light" | "dark") => {
    await waitFor(() => expect(sent[0]).toBeDefined())
    deliver({
      id: sent[0].id as number,
      result: { hostContext: { theme, containerDimensions: { height: 320 } } },
    })
    await waitFor(() => expect(sent.length).toBeGreaterThan(1))
  }
  return { parent, sent, deliver, handshake }
}

describe("the recorded show_panel result", () => {
  it("is a payload the panel accepts", () => {
    expect(payloadOf(recorded)?.title).toBe("Requests per second by route")
  })

  it("is a problem when the tool failed", () => {
    expect(
      viewOf({ isError: true, content: [{ type: "text", text: "no such" }] })
    ).toEqual({ kind: "problem", message: "no such" })
  })
})

describe("McpPanel", () => {
  it("draws the recorded result with the host theme and makes no request", async () => {
    const fake = host()
    const { container } = render(<McpPanel target={fake.parent} />)
    expect(screen.getByRole("status")).toHaveTextContent("Waiting")
    await fake.handshake("dark")
    expect(document.documentElement.classList.contains("dark")).toBe(true)

    fake.deliver({
      method: "ui/notifications/tool-result",
      params: asStat,
    })
    expect(
      await screen.findByRole("heading", {
        name: "Requests per second by route",
      })
    ).toBeInTheDocument()
    expect(screen.getByText("now-1h to now")).toBeInTheDocument()
    expect(await screen.findByText('{route="/checkout"}')).toBeInTheDocument()
    expect(screen.queryByText("Could not load this panel")).toBeNull()
    expect(container.querySelector("section")).toHaveAttribute(
      "aria-label",
      "Requests per second by route"
    )
    expect(Object.values(network).map((spy) => spy.mock.calls.length)).toEqual([
      0, 0, 0, 0, 0,
    ])
  })

  it("asks the host to open the panel in SlashObserve", async () => {
    const fake = host()
    render(<McpPanel target={fake.parent} />)
    await fake.handshake("light")
    fake.deliver({ method: "ui/notifications/tool-result", params: recorded })
    const button = await screen.findByRole("button", {
      name: /Open in SlashObserve/,
    })
    act(() => button.click())
    expect(fake.sent[fake.sent.length - 1]).toMatchObject({
      method: "ui/open-link",
      params: { url: recorded.structuredContent.open_url },
    })
  })

  it("tells the person what went wrong when the tool failed", async () => {
    const fake = host()
    render(<McpPanel target={fake.parent} />)
    await fake.handshake("light")
    fake.deliver({
      method: "ui/notifications/tool-result",
      params: {
        isError: true,
        content: [{ type: "text", text: "unknown datasource" }],
      },
    })
    expect(await screen.findByRole("status")).toHaveTextContent(
      "unknown datasource"
    )
  })
})

describe("chartHeightFor", () => {
  it("leaves room for the frame inside a fixed height", () => {
    expect(chartHeightFor({ height: 320 })).toBe(312)
  })

  it("stays within a maximum height when no height is given", () => {
    expect(chartHeightFor({ maxHeight: 200 })).toBe(192)
    expect(chartHeightFor({ maxHeight: 900 })).toBe(360)
  })

  it("uses the default height when the host gives no limit", () => {
    expect(chartHeightFor(undefined)).toBe(360)
    expect(chartHeightFor({ width: 500 })).toBe(360)
  })
})
