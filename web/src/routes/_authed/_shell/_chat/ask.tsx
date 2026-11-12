import { createFileRoute } from "@tanstack/react-router"

import { ChatHome } from "@/chat/chat-home"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/_chat/ask")({
  staticData: { crumbs: [{ label: "Ask" }] },
  component: Ask,
})

function Ask() {
  useDocumentTitle("Ask - SlashObserve")
  return <ChatHome />
}
