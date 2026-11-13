import { createFileRoute } from "@tanstack/react-router"

import type { InboxView } from "@/api/keys"
import { InboxPage, isInboxView } from "@/inbox/inbox-page"
import { useDocumentTitle } from "@/lib/document-title"

type InboxSearch = { view?: InboxView; item?: string }

const seqPattern = /^[0-9]{1,18}$/

export const Route = createFileRoute("/_authed/_shell/inbox")({
  staticData: { crumbs: [{ label: "Inbox" }] },
  validateSearch: (search: Record<string, unknown>): InboxSearch => {
    const item = String(search.item ?? "")
    return {
      view:
        isInboxView(search.view) && search.view !== "for_you"
          ? search.view
          : undefined,
      item: seqPattern.test(item) ? item : undefined,
    }
  },
  component: InboxRoute,
})

function InboxRoute() {
  useDocumentTitle("Inbox - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <InboxPage
      view={search.view ?? "for_you"}
      selected={search.item}
      onChange={(next) =>
        void navigate({
          search: {
            ...(next.view === "for_you" ? {} : { view: next.view }),
            ...(next.item === undefined ? {} : { item: next.item }),
          },
          replace: true,
        })
      }
    />
  )
}
