import {
  useEffect,
  useId,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

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
import { Button } from "@/components/ui/button"
import { dashboardLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"

import { mergeLeftProposalStale, mergeProblem } from "./merge-problem"
import { choicesFrom, unitKey, type Picks } from "./proposal-view"
import {
  proposalQuery,
  useDiscardProposal,
  useMergeProposal,
  type Proposal,
} from "./queries"

type Props = {
  proposal: Proposal
  picks: Picks
  onPicksChange: Dispatch<SetStateAction<Picks>>
  onMergingChange?: (merging: boolean) => void
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

export function ProposalActions({
  proposal,
  picks,
  onPicksChange,
  onMergingChange,
}: Props) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const hintId = useId()
  const merge = useMergeProposal(proposal.id)
  const [problem, setProblem] = useState<string | null>(null)
  const merging = merge.isPending
  useEffect(() => {
    onMergingChange?.(merging)
  }, [merging, onMergingChange])
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

  function refetch() {
    const query = proposalQuery(proposal.id)
    void queryClient
      .refetchQueries({ queryKey: query.queryKey, exact: true })
      .then(() => {
        const fresh = queryClient.getQueryData(query.queryKey)
        const present = new Set((fresh?.conflicts ?? []).map(unitKey))
        onPicksChange((current) =>
          Object.fromEntries(
            Object.entries(current).filter(([key]) => present.has(key))
          )
        )
      })
  }

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
          if (mergeLeftProposalStale(error)) refetch()
        },
      }
    )
  }

  if (!canMerge && !canDiscard) return null

  return (
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
          <DiscardButton proposal={proposal} onStale={refetch} />
        ) : null}
      </div>
      {hint === null ? null : (
        <p id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </p>
      )}
    </div>
  )
}
