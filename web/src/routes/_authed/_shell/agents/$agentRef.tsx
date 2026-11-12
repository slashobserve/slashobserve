import { createFileRoute } from "@tanstack/react-router"

import { AgentDetailPage } from "@/agents/agent-detail-page"
import { parseAgentRef } from "@/agents/agent-ref"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/agents/$agentRef")({
  staticData: {
    crumbs: [{ label: "Agents", to: "/agents" }, { label: "Agent" }],
  },
  component: AgentRoute,
})

function AgentRoute() {
  useDocumentTitle("Agent - SlashObserve")
  const { agentRef } = Route.useParams()
  return <AgentDetailPage key={agentRef} agentRef={parseAgentRef(agentRef)} />
}
