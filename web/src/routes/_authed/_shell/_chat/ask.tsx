import { createFileRoute } from "@tanstack/react-router"

import { parseAskSearch, type AskSearch } from "@/chat/ask-search"
import { ChatHome } from "@/chat/chat-home"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/_chat/ask")({
  validateSearch: (search: Record<string, unknown>): AskSearch =>
    parseAskSearch(search),
  staticData: { crumbs: [{ label: "Ask" }] },
  component: Ask,
})

function Ask() {
  useDocumentTitle("Ask - SlashObserve")
  const { prompt, dashboard } = Route.useSearch()
  return <ChatHome initialPrompt={prompt} dashboardId={dashboard} />
}
