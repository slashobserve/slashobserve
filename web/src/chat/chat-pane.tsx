import { useId, useRef } from "react"

import { useActivityStore, useChatPaneLayout } from "@/activity/activity-store"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"

import { Conversation } from "./conversation"

export function ChatPane({
  requestId,
  onClose,
}: {
  requestId: string
  onClose?: () => void
}) {
  const { bounds, width, stacked } = useChatPaneLayout()
  const setChatWidth = useActivityStore((state) => state.setChatWidth)
  const headingId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  return (
    <DockPanel
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
        onClose={onClose}
        closeLabel="Close panel"
      />
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pt-3">
        <Conversation key={requestId} requestId={requestId} layout="pane" />
      </div>
    </DockPanel>
  )
}
