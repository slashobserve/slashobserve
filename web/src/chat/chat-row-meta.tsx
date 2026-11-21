import { LayoutDashboardIcon } from "lucide-react"

import { dashboardLabel, turnsText, type DashboardTitles } from "./chat-rows"
import type { Chat } from "./chats-queries"

export function ChatRowMeta({
  chat,
  titles,
}: {
  chat: Chat
  titles: DashboardTitles | undefined
}) {
  const dashboard = dashboardLabel(chat.dashboard_id, titles)
  return (
    <>
      <span>{turnsText(chat.turns)}</span>
      {dashboard === null ? null : (
        <span className="inline-flex h-5 max-w-full min-w-0 items-center gap-1 rounded-chip bg-muted px-1.5 text-xs font-medium text-foreground">
          <LayoutDashboardIcon
            aria-hidden="true"
            className="size-3 flex-none text-muted-foreground"
          />
          <span className="truncate">{dashboard}</span>
        </span>
      )}
    </>
  )
}
