import { createFileRoute } from "@tanstack/react-router"

import { ChatsPage } from "@/chat/chats-page"
import { parseChatsSearch } from "@/chat/chats-search"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/chats")({
  staticData: { crumbs: [{ label: "Chats" }] },
  validateSearch: parseChatsSearch,
  component: ChatsRoute,
})

function ChatsRoute() {
  useDocumentTitle("Chats - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ChatsPage
      search={search}
      onSearch={(next) => void navigate({ search: next, replace: true })}
    />
  )
}
