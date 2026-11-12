import { useId, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { latestDashboardQuery } from "@/dashboards/queries"
import { Discussion } from "@/discussion/discussion"
import { useDocumentTitle } from "@/lib/document-title"
import { dashboardLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import type { Side } from "./conflict-choices"
import { mergeLeftProposalStale, mergeProblem } from "./merge-problem"
import { ProposalBody } from "./proposal-body"
import { choicesFrom, unitKey, type Picks } from "./proposal-view"
import {
  proposalQuery,
  useDiscardProposal,
  useMergeProposal,
  type Proposal,
} from "./queries"

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

function DiscardButton({
  proposal,
  onStale,
}: {
  proposal: Proposal
  onStale: () => void
}) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const discard = useDiscardProposal(proposal.id)

  function changeOpen(next: boolean) {
    if (!next && discard.isPending) return
    setOpen(next)
    if (!next) {
      setProblem(null)
      discard.reset()
    }
  }

  function confirm() {
    setProblem(null)
    discard.mutate(undefined, {
      onSuccess: () => {
        toast.success("Discarded")
        setOpen(false)
      },
      onError: (error) => {
        setProblem(loadProblemText(error))
        if (mergeLeftProposalStale(error)) onStale()
      },
    })
  }

  return (
    <>
      <Button variant="outline" onClick={() => setOpen(true)}>
        Discard
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard this proposal?</AlertDialogTitle>
            <AlertDialogDescription>
              Its change is not saved.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem === null ? null : (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={discard.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={discard.isPending}
              focusableWhenDisabled
              aria-busy={discard.isPending}
              onClick={confirm}
            >
              {discard.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Discard
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function LoadedProposal({
  proposal,
  onStale,
  picks,
  onPick,
}: {
  proposal: Proposal
  onStale: () => void
  picks: Picks
  onPick: (key: string, side: Side) => void
}) {
  useDocumentTitle(`${proposal.title} - Proposals - SlashObserve`)
  const navigate = useNavigate()
  const hintId = useId()
  const merge = useMergeProposal(proposal.id)
  const [problem, setProblem] = useState<string | null>(null)
  const canMerge = proposal.allowed_actions.includes("merge")
  const canDiscard = proposal.allowed_actions.includes("discard")
  const { choices, missing } = choicesFrom(proposal.conflicts, picks)
  const blocked = proposal.conflicts.some(
    (conflict) => conflict.reason === "invalid"
  )
  const hint = blocked
    ? "Some changes cannot be merged; discard this proposal or ask its author to fix it"
    : missing.length > 0
      ? "Choose a side for each conflict first"
      : null

  function submit() {
    if (missing.length > 0 || merge.isPending) return
    setProblem(null)
    merge.mutate(
      { revision: proposal.revision, choices },
      {
        onSuccess: (merged) => {
          toast.success(
            merged.merged_version === null
              ? "Merged"
              : `Merged as version ${merged.merged_version}`,
            {
              action: {
                label: "Open dashboard",
                onClick: () =>
                  void navigate({ ...dashboardLink(merged.dashboard_id) }),
              },
            }
          )
        },
        onError: (error) => {
          setProblem(mergeProblem(error))
          if (mergeLeftProposalStale(error)) onStale()
        },
      }
    )
  }

  return (
    <div className={columnClass}>
      <Location proposal={proposal} />
      <ProposalBody
        proposal={proposal}
        picks={picks}
        onPick={onPick}
        disabled={merge.isPending}
      />
      {canMerge || canDiscard ? (
        <div className="flex flex-col gap-2">
          {problem === null ? null : (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          )}
          <div className="flex items-center gap-2">
            {canMerge ? (
              <Button
                aria-disabled={hint !== null || undefined}
                aria-describedby={hint === null ? undefined : hintId}
                aria-busy={merge.isPending || undefined}
                onClick={submit}
              >
                {merge.isPending ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="motion-safe:animate-spin"
                  />
                ) : null}
                Merge
              </Button>
            ) : null}
            {canDiscard ? (
              <DiscardButton proposal={proposal} onStale={onStale} />
            ) : null}
          </div>
          {hint === null ? null : (
            <p id={hintId} className="text-xs text-muted-foreground">
              {hint}
            </p>
          )}
        </div>
      ) : null}
    </div>
  )
}

export function ProposalPage({ proposalId }: { proposalId: string }) {
  const query = useQuery(proposalQuery(proposalId))
  const [picks, setPicks] = useState<Picks>({})
  const proposal = query.data

  function pick(key: string, side: Side) {
    setPicks((current) => ({ ...current, [key]: side }))
  }

  function refetch() {
    void query.refetch().then((result) => {
      const present = new Set((result.data?.conflicts ?? []).map(unitKey))
      setPicks((current) =>
        Object.fromEntries(
          Object.entries(current).filter(([key]) => present.has(key))
        )
      )
    })
  }

  if (proposal !== undefined) {
    return (
      <div className="@container">
        <div className="flex flex-col gap-6 @min-[1100px]:flex-row @min-[1100px]:items-start">
          <div className="min-w-0 flex-1">
            <LoadedProposal
              proposal={proposal}
              picks={picks}
              onPick={pick}
              onStale={refetch}
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
