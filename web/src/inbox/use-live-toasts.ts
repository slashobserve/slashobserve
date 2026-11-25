import { useEffect } from "react"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { plainTitle } from "@/comments/plain-title"

import { itemLink, itemText, type InboxItem } from "./inbox-item"
import { useInboxPanelStore } from "./inbox-panel-store"
import { liveToastDuration, liveToasterId, planLiveToasts } from "./live-toasts"
import { useInboxForYou, useSetInboxItemUnread } from "./queries"

let seen: number | null = null

export function resetLiveToastsSeen(): void {
  seen = null
}

function toastTitle(item: InboxItem): string {
  const { actor, verb, object } = itemText(item)
  return `${actor === "" ? "" : `${actor} `}${verb} ${object}`
}

function toastDescription(item: InboxItem): string | undefined {
  if (item.target_title === null || item.target_title === "") return undefined
  const title = plainTitle(item.target_title)
  return toastTitle(item).includes(title) ? undefined : title
}

export function useLiveInboxToasts(): void {
  const navigate = useNavigate()
  const { mutate: markRead } = useSetInboxItemUnread()
  const data = useInboxForYou().data

  useEffect(() => {
    if (data === undefined) return
    const items = data.pages.flatMap((page) => page.items)
    const plan = planLiveToasts(items, seen, {
      panelOpen: useInboxPanelStore.getState().open,
      visible: !document.hidden,
    })
    seen = plan.seen

    function open(item: InboxItem) {
      if (item.unread) markRead({ seq: item.seq, unread: false })
      const link = itemLink(item)
      if (link !== null) void navigate({ to: link.to, search: link.search })
    }

    for (const item of plan.items) {
      toast(toastTitle(item), {
        toasterId: liveToasterId,
        duration: liveToastDuration,
        description: toastDescription(item),
        action: { label: "Open", onClick: () => open(item) },
      })
    }
    if (plan.overflow > 0) {
      toast(`${plan.overflow} new in Inbox`, {
        toasterId: liveToasterId,
        duration: liveToastDuration,
        action: {
          label: "Open",
          onClick: () => useInboxPanelStore.getState().show(),
        },
      })
    }
  }, [data, navigate, markRead])
}
