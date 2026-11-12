import { useState, type Dispatch, type SetStateAction } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { latestDashboardQuery } from "@/dashboards/queries"
import { Discussion } from "@/discussion/discussion"
import { useDocumentTitle } from "@/lib/document-title"
import { dashboardLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import type { Side } from "./conflict-choices"
import { ProposalActions } from "./proposal-actions"
import { ProposalBody } from "./proposal-body"
import type { Picks } from "./proposal-view"
import { proposalQuery, type Proposal } from "./queries"

const columnClass = "flex w-full max-w-[720px] min-w-0 flex-col gap-6"

function ProposalSkeleton() {
  return (
    <div aria-busy="true" className={columnClass}>
      <Skeleton className="h-7 w-1/2" />
      <div className="flex flex-col gap-3">
        {[0, 1, 2].map((row) => (
          <Skeleton key={row} className="h-10 rounded-card" />
        ))}
      </div>
    </div>
  )
}

function ProposalNotFound() {
  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-4 text-center">
      <h1 className="text-2xl font-semibold">Proposal not found</h1>
      <p className="text-sm text-muted-foreground">
        This proposal does not exist or you cannot see it
      </p>
      <Link
        to="/dashboards"
        className={cn(buttonVariants({ variant: "outline" }))}
      >
        Back to Dashboards
      </Link>
    </div>
  )
}

function Location({ proposal }: { proposal: Proposal }) {
  const queryClient = useQueryClient()
  const latest = useQuery(
    latestDashboardQuery(queryClient, proposal.dashboard_id)
  )
  const title = latest.data?.dashboard.title
  return (
    <nav
      aria-label="Proposal location"
      className="flex min-w-0 items-center gap-2 text-[13px]"
    >
      <Link
        {...dashboardLink(proposal.dashboard_id, title)}
        className="rounded-control text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        {title ?? "Dashboard"}
      </Link>
      <span aria-hidden="true" className="text-muted-foreground">
        /
      </span>
      <span aria-current="page" className="font-medium">
        Proposals
      </span>
    </nav>
  )
}

function LoadedProposal({
  proposal,
  picks,
  onPicksChange,
}: {
  proposal: Proposal
  picks: Picks
  onPicksChange: Dispatch<SetStateAction<Picks>>
}) {
  useDocumentTitle(`${proposal.title} - Proposals - SlashObserve`)
  const [merging, setMerging] = useState(false)

  function pick(key: string, side: Side) {
    onPicksChange((current) => ({ ...current, [key]: side }))
  }

  return (
    <div className={columnClass}>
      <Location proposal={proposal} />
      <ProposalBody
        proposal={proposal}
        picks={picks}
        onPick={pick}
        disabled={merging}
      />
      <ProposalActions
        proposal={proposal}
        picks={picks}
        onPicksChange={onPicksChange}
        onMergingChange={setMerging}
      />
    </div>
  )
}

export function ProposalPage({ proposalId }: { proposalId: string }) {
  const query = useQuery(proposalQuery(proposalId))
  const [picks, setPicks] = useState<Picks>({})
  const proposal = query.data

  if (proposal !== undefined) {
    return (
      <div className="@container">
        <div className="flex flex-col gap-6 @min-[1100px]:flex-row @min-[1100px]:items-start">
          <div className="min-w-0 flex-1">
            <LoadedProposal
              proposal={proposal}
              picks={picks}
              onPicksChange={setPicks}
            />
          </div>
          <div className="w-full @min-[1100px]:sticky @min-[1100px]:top-3.5 @min-[1100px]:h-[calc(100svh-4.75rem)] @min-[1100px]:w-96 @min-[1100px]:flex-none">
            <Discussion
              className="h-full max-h-[480px] @min-[1100px]:max-h-none"
              target={`proposal:${proposal.id}`}
              hint="Discuss this change with its author"
              emptyText="No messages yet."
            />
          </div>
        </div>
      </div>
    )
  }
  if (query.error instanceof ApiError && query.error.status === 404) {
    return <ProposalNotFound />
  }
  if (query.isError) {
    return (
      <div className={columnClass}>
        <LoadProblem
          title="Could not load this proposal"
          detail={sentence(query.error.message)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </div>
    )
  }
  return <ProposalSkeleton />
}
