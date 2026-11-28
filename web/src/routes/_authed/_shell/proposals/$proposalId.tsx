import { createFileRoute } from "@tanstack/react-router"

import { threadParam } from "@/discussion/thread-param"
import { ProposalPage } from "@/proposals/proposal-page"

export const Route = createFileRoute("/_authed/_shell/proposals/$proposalId")({
  staticData: {
    crumbs: [{ label: "Dashboards", to: "/dashboards" }, { label: "Proposal" }],
  },
  validateSearch: (
    search: Record<string, unknown>
  ): { thread?: string | undefined } => ({
    thread: threadParam(search.thread),
  }),
  component: ProposalRoute,
})

function ProposalRoute() {
  const { proposalId } = Route.useParams()
  const { thread } = Route.useSearch()
  return (
    <ProposalPage
      key={proposalId}
      proposalId={proposalId}
      focusedThreadId={thread}
    />
  )
}
