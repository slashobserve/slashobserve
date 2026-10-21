import { createFileRoute } from "@tanstack/react-router"

import { ChatHome } from "@/chat/chat-home"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/_chat/")({
  staticData: { crumbs: [{ label: "Observe" }] },
  component: Home,
})

function Home() {
  useDocumentTitle("Observe - SlashObserve")
  return <ChatHome />
}
