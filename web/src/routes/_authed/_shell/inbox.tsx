import { createFileRoute } from "@tanstack/react-router"

import type { InboxFilter, InboxView } from "@/api/keys"
import { InboxPage } from "@/inbox/inbox-page"
import { isInboxFilter, isInboxView } from "@/inbox/inbox-views"
import { useDocumentTitle } from "@/lib/document-title"

type InboxSearch = {
  view?: InboxView
  filter?: InboxFilter
  group?: true
  item?: string
}

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
      filter:
        isInboxFilter(search.filter) && search.filter !== "open"
          ? search.filter
          : undefined,
      group:
        search.group === true || search.group === "true" ? true : undefined,
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
      filter={search.filter ?? "open"}
      grouped={search.group === true}
      selected={search.item}
      onChange={(next) =>
        void navigate({
          search: {
            ...(next.view === "for_you" ? {} : { view: next.view }),
            ...(next.filter === "open" ? {} : { filter: next.filter }),
            ...(next.grouped ? { group: true as const } : {}),
            ...(next.item === undefined ? {} : { item: next.item }),
          },
          replace: true,
        })
      }
    />
  )
}
