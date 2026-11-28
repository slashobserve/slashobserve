import { useEffect } from "react"
import { useMatch, useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { plainTitle } from "@/comments/plain-title"
import { useDocumentVisible } from "@/live/use-activity-stream"

import { itemLink, itemText, type InboxItem } from "./inbox-item"
import { useInboxPanelStore } from "./inbox-panel-store"
import {
  gateLiveToasts,
  liveToastDuration,
  liveToasterId,
  planLiveToasts,
} from "./live-toasts"
import { useInboxForYou, useSetInboxItemUnread } from "./queries"

type Cursor = { principal: string | null; seen: number | null }

const freshCursor: Cursor = { principal: null, seen: null }

let cursor: Cursor = freshCursor

export function resetLiveToasts(): void {
  cursor = freshCursor
}

function cursorFor(principal: string | null): Cursor {
  if (cursor.principal !== principal) cursor = { ...freshCursor, principal }
  return cursor
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
  const { data } = useInboxForYou()
  const visible = useDocumentVisible()
  const principal =
    useMatch({
      from: "/_authed",
      shouldThrow: false,
      select: (match) => match.context.principal.id,
    }) ?? null

  useEffect(() => {
    if (data === undefined) return
    const items = data.pages.flatMap((page) => page.items)
    const newest = items.reduce((top, item) => Math.max(top, item.seq), 0)
    const current = cursorFor(principal)
    const gate = gateLiveToasts(newest, current.seen, {
      panelOpen: useInboxPanelStore.getState().open,
      hidden: !visible,
    })
    current.seen = gate.seen
    if (!gate.fresh) return
    const plan = planLiveToasts(items, gate.seen)
    current.seen = plan.seen

    function open(item: InboxItem) {
      if (item.unread) markRead({ seq: item.seq, unread: false })
      const link = itemLink(item)
      if (link === null) {
        useInboxPanelStore
          .getState()
          .show({ view: "for_you", filter: "open", item: item.seq })
        return
      }
      void navigate({
        to: link.to,
        ...(link.search === undefined ? {} : { search: link.search }),
      })
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
  }, [data, visible, principal, navigate, markRead])
}
