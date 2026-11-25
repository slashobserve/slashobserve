import { useQuery } from "@tanstack/react-query"
import { useId, useRef } from "react"

import { useActivityStore, useChatPaneLayout } from "@/activity/activity-store"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { useDockFocus } from "@/components/dock/dock-focus"
import { runStateText } from "@/observe/build-page-model"
import { latestTurn } from "@/observe/conversation"
import { useObserveRequest } from "@/observe/queries"
import { revisionsQuery } from "@/revisions/queries"

import { Conversation } from "./conversation"

export function ChatPane({
  requestId,
  onClose,
  opener,
}: {
  requestId: string
  onClose?: () => void
  opener?: () => HTMLElement | null
}) {
  const { bounds, width, stacked } = useChatPaneLayout()
  const setChatWidth = useActivityStore((state) => state.setChatWidth)
  const headingId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const pane = useRef<HTMLElement>(null)
  const { close } = useDockFocus({ pane, heading, opener, onClose })
  const request = useObserveRequest(requestId).data
  const dashboardId = request?.dashboard_id ?? null
  const revisions = useQuery({
    ...revisionsQuery(dashboardId ?? ""),
    enabled: dashboardId !== null && request?.read_only !== true,
  })
  const latest =
    request === undefined ? undefined : latestTurn(request, revisions.data)
  return (
    <DockPanel
      panelKey="chat"
      ref={pane}
      labelledBy={headingId}
      width={width}
      bounds={bounds}
      onWidthChange={setChatWidth}
      stacked={stacked}
    >
      <DockHeader
        headingId={headingId}
        headingRef={heading}
        title="Chat"
        description={latest === undefined ? undefined : runStateText(latest)}
        onClose={onClose === undefined ? undefined : close}
        closeLabel="Close panel"
      />
      <div className="flex min-h-0 flex-1 flex-col px-4 pt-3">
        <Conversation key={requestId} requestId={requestId} layout="pane" />
      </div>
    </DockPanel>
  )
}
