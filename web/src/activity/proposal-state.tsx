import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { proposalLink } from "@/lib/links"
import { runProposalQuery, type ProposalSummary } from "@/proposals/queries"

import { MetaState } from "./entry-frame"

export function proposalStateText(proposal: ProposalSummary | undefined) {
  switch (proposal?.state) {
    case "merged":
      return proposal.merged_version === null
        ? "Proposal merged"
        : `Proposal merged as version ${proposal.merged_version}`
    case "discarded":
      return "Proposal discarded"
    case "open":
      return "Proposal opened, waiting for an Editor"
    default:
      return "Proposal"
  }
}

export function RunProposalState({
  dashboardId,
  requestId,
}: {
  dashboardId: string
  requestId: string
}) {
  const proposal = useQuery(runProposalQuery(dashboardId, requestId)).data
    ?.proposals[0]
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
