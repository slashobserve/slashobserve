import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { proposalLink } from "@/lib/links"
import {
  dashboardProposalsQuery,
  type ProposalSummary,
} from "@/proposals/queries"

import { MetaState } from "./entry-frame"

export function proposalStateText(proposal: ProposalSummary | undefined) {
  switch (proposal?.state) {
    case "merged":
      return proposal.merged_version === null
        ? "Proposal merged"
        : `Proposal merged as version ${proposal.merged_version}`
    case "discarded":
      return "Proposal discarded"
    default:
      return "Proposal opened, waiting for an Editor"
  }
}

export function RunProposalState({
  dashboardId,
  requestId,
}: {
  dashboardId: string
  requestId: string
}) {
  const proposals = useQuery(dashboardProposalsQuery(dashboardId, "all"))
  const proposal = proposals.data?.proposals.find(
    (item) => item.run_id === requestId
  )
  const text = proposalStateText(proposal)
  if (proposal === undefined) return <MetaState>{text}</MetaState>
  return (
    <Link
      {...proposalLink(proposal.id)}
      className="rounded-sm text-foreground underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
    >
      <MetaState>{text}</MetaState>
    </Link>
  )
}
