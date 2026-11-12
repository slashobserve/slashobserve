import { createFileRoute } from "@tanstack/react-router"

import { ProposalPage } from "@/proposals/proposal-page"

export const Route = createFileRoute("/_authed/_shell/proposals/$proposalId")({
  staticData: {
    crumbs: [{ label: "Dashboards", to: "/dashboards" }, { label: "Proposal" }],
  },
  component: ProposalRoute,
})

function ProposalRoute() {
  const { proposalId } = Route.useParams()
  return <ProposalPage key={proposalId} proposalId={proposalId} />
}
