import type { CSSProperties } from "react"

import { useActivityStore, useChatPaneLayout } from "@/activity/activity-store"
import { ResizeHandle } from "@/activity/resize-handle"
import { cn } from "@/lib/utils"

import { Conversation } from "./conversation"

export function ChatPane({ requestId }: { requestId: string }) {
  const { bounds, width, stacked } = useChatPaneLayout()
  const setChatWidth = useActivityStore((state) => state.setChatWidth)
  return (
    <aside
      aria-label="Chat"
      style={
        stacked ? undefined : ({ "--chat-w": `${width}px` } as CSSProperties)
      }
      className={cn(
        "relative flex min-h-0 flex-col",
        stacked ? "h-[70svh] border-t" : "flex-[0_0_var(--chat-w)] border-r"
      )}
    >
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pt-3">
        <Conversation key={requestId} requestId={requestId} layout="pane" />
      </div>
      {stacked ? null : (
        <ResizeHandle
          side="right"
          width={width}
          bounds={bounds}
          onChange={setChatWidth}
        />
      )}
    </aside>
  )
}
