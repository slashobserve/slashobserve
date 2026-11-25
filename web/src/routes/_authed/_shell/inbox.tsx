import { createFileRoute, redirect } from "@tanstack/react-router"

import { useInboxPanelStore } from "@/inbox/inbox-panel-store"
import { inboxChoice } from "@/inbox/inbox-views"

export const Route = createFileRoute("/_authed/_shell/inbox")({
  beforeLoad: ({ search, preload }) => {
    if (!preload) {
      useInboxPanelStore.getState().showAfterNavigation(inboxChoice(search))
    }
    throw redirect({ to: "/", replace: true })
  },
})
