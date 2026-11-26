import type { ToolResult } from "./payload"

export const protocolVersion = "2026-01-26"

type Theme = "light" | "dark"

type ContainerDimensions = {
  width?: number
  maxWidth?: number
  height?: number
  maxHeight?: number
}

export type HostContext = {
  theme?: Theme
  locale?: string
  timeZone?: string
  containerDimensions?: ContainerDimensions
}

export type HostHandlers = {
  onToolResult: (result: ToolResult) => void
  onToolCancelled: () => void
  onContext: (context: HostContext) => void
}

export type Host = {
  context: () => HostContext
  openLink: (url: string) => Promise<void>
  reportSize: (width: number, height: number) => void
  disconnect: () => void
}

type Message = {
  jsonrpc?: string
  id?: number | string
  method?: string
  params?: unknown
  result?: unknown
  error?: { code?: number; message?: string }
}

const methodNotFound = -32601

function isMessage(data: unknown): data is Message {
  return (
    typeof data === "object" &&
    data !== null &&
    (data as Message).jsonrpc === "2.0"
  )
}

export function connect(
  target: Window,
  self: Window,
  handlers: HostHandlers
): Promise<Host> {
  let next = 1
  let context: HostContext = {}
  const pending = new Map<
    number | string,
    { resolve: (value: unknown) => void; reject: (error: Error) => void }
  >()

  const post = (message: Message) =>
    target.postMessage({ jsonrpc: "2.0", ...message }, "*")

  const request = (method: string, params: unknown) =>
    new Promise<unknown>((resolve, reject) => {
      const id = next++
      pending.set(id, { resolve, reject })
      post({ id, method, params })
    })

  const notify = (method: string, params: unknown) => post({ method, params })

  const listener = (event: MessageEvent) => {
    if (event.source !== target || !isMessage(event.data)) return
    const message = event.data
    if (message.method === undefined) {
      if (message.id === undefined) return
      const waiting = pending.get(message.id)
      pending.delete(message.id)
      if (message.error) {
        waiting?.reject(new Error(message.error.message ?? "request failed"))
      } else waiting?.resolve(message.result)
      return
    }
    switch (message.method) {
      case "ui/notifications/tool-result":
        handlers.onToolResult((message.params ?? {}) as ToolResult)
        return
      case "ui/notifications/tool-cancelled":
        handlers.onToolCancelled()
        return
      case "ui/notifications/host-context-changed":
        context = { ...context, ...(message.params as HostContext) }
        handlers.onContext(context)
        return
    }
    if (message.id === undefined) return
    if (
      message.method === "ping" ||
      message.method === "ui/resource-teardown"
    ) {
      post({ id: message.id, result: {} })
      return
    }
    post({
      id: message.id,
      error: { code: methodNotFound, message: "method not found" },
    })
  }
  self.addEventListener("message", listener)

  return request("ui/initialize", {
    appInfo: { name: "slashobserve-panel", version: "1.0.0" },
    appCapabilities: { availableDisplayModes: ["inline"] },
    protocolVersion,
  }).then((result) => {
    context = ((result as { hostContext?: HostContext } | undefined)
      ?.hostContext ?? {}) as HostContext
    notify("ui/notifications/initialized", {})
    handlers.onContext(context)
    return {
      context: () => context,
      openLink: (url) => request("ui/open-link", { url }).then(() => undefined),
      reportSize: (width, height) =>
        notify("ui/notifications/size-changed", { width, height }),
      disconnect: () => self.removeEventListener("message", listener),
    }
  })
}
