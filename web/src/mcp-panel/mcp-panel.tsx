import { useEffect, useRef, useState } from "react"

import { useTheme } from "@/lib/theme"

import { connect, type Host, type HostContext } from "./host"
import { PanelApp } from "./panel-app"
import {
  failureOf,
  payloadOf,
  type PanelPayload,
  type ToolResult,
} from "./payload"

type View =
  | { kind: "waiting" }
  | { kind: "ready"; payload: PanelPayload }
  | { kind: "problem"; message: string }

const chartHeight = 360
const framePadding = 8
const fallbackWidth = 640

export function chartHeightFor(
  dimensions?: HostContext["containerDimensions"]
) {
  if (dimensions?.height !== undefined) {
    return Math.max(0, dimensions.height - framePadding)
  }
  if (dimensions?.maxHeight !== undefined) {
    return Math.max(
      0,
      Math.min(chartHeight, dimensions.maxHeight - framePadding)
    )
  }
  return chartHeight
}

export function viewOf(result: ToolResult): View {
  const payload = payloadOf(result)
  return payload
    ? { kind: "ready", payload }
    : { kind: "problem", message: failureOf(result) }
}

type Props = { target?: Window; self?: Window }

export function McpPanel({ target = window.parent, self = window }: Props) {
  const [view, setView] = useState<View>({ kind: "waiting" })
  const [context, setContext] = useState<HostContext>({})
  const [width, setWidth] = useState(self.innerWidth || fallbackWidth)
  const [openFailed, setOpenFailed] = useState(false)
  const host = useRef<Host>(undefined)
  const body = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let live = true
    let connected: Host | undefined
    connect(target, self, {
      onToolResult: (result) => setView(viewOf(result)),
      onToolCancelled: () =>
        setView({ kind: "problem", message: "The query was cancelled." }),
      onContext: setContext,
    }).then(
      (h) => {
        if (!live) return h.disconnect()
        connected = h
        host.current = h
      },
      () => setView({ kind: "problem", message: "The host did not answer." })
    )
    return () => {
      live = false
      connected?.disconnect()
      host.current = undefined
    }
  }, [target, self])

  const { setPreference } = useTheme()
  useEffect(() => {
    setPreference(context.theme ?? "system")
  }, [context.theme, setPreference])

  useEffect(() => {
    const element = body.current
    if (!element) return
    const observer = new ResizeObserver(() => {
      const box = element.getBoundingClientRect()
      if (box.width > 0) setWidth(Math.floor(box.width))
      host.current?.reportSize(Math.ceil(box.width), Math.ceil(box.height))
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [view.kind])

  const height = chartHeightFor(context.containerDimensions)

  const open = (url: string) => {
    setOpenFailed(false)
    host.current?.openLink(url).catch(() => {
      if (self.open(url, "_blank", "noopener") === null) setOpenFailed(true)
    })
  }

  return (
    <div ref={body} className="w-full p-1 text-foreground">
      {view.kind === "ready" ? (
        <PanelApp
          payload={view.payload}
          width={width - framePadding}
          height={height}
          onOpen={open}
        />
      ) : (
        <p className="p-3 text-sm text-muted-foreground" role="status">
          {view.kind === "waiting" ? "Waiting for the panel." : view.message}
        </p>
      )}
      {openFailed ? (
        <p className="px-1 text-xs text-muted-foreground" role="status">
          The host did not open the link.
        </p>
      ) : null}
    </div>
  )
}
