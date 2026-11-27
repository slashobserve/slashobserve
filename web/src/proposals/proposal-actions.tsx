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
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { withWarning } from "@/dashboards/save-outcome"
import { dashboardLink } from "@/lib/links"
import { loadProblemText } from "@/service-accounts/queries"

import { mergeLeftProposalStale, mergeProblem } from "./merge-problem"
import {
  choicesFrom,
  droppedText,
  mergeHint,
  unitKey,
  type Picks,
} from "./proposal-view"
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

const maxReasonLength = 500

const whoMergesText =
  "Only someone who can edit this dashboard can merge a proposal."
const authorDiscardText = `${whoMergesText} You can discard this one because you opened it.`

function DiscardButton({
  proposal,
  onStale,
}: {
  proposal: Proposal
  onStale: () => void
}) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const [reason, setReason] = useState("")
  const reasonId = useId()
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
    discard.mutateAsync(reason.trim()).then(
      () => {
        toast.success("Discarded")
        setOpen(false)
        setReason("")
      },
      (error) => {
        setProblem(loadProblemText(error))
        if (mergeLeftProposalStale(error)) onStale()
      }
    )
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
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={reasonId}>Reason (optional)</Label>
            <Textarea
              id={reasonId}
              rows={2}
              maxLength={maxReasonLength}
              value={reason}
              placeholder="Shown on the proposal"
              disabled={discard.isPending}
              onChange={(event) => setReason(event.target.value)}
            />
          </div>
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
  const hint = mergeHint(proposal, missing)

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
    if (hint !== null || merge.isPending) return
    setProblem(null)
    merge.mutate(
      { revision: proposal.revision, choices },
      {
        onSuccess: (merged) => {
          toast.success(
            merged.merged_version === null
              ? "Merged without a new version"
              : withWarning(
                  `Merged as version ${merged.merged_version}`,
                  merged.warning
                ),
            {
              description: droppedText(proposal, picks) ?? undefined,
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

  if (proposal.state !== "open") return null

  if (!canMerge && !canDiscard) {
    return <p className="text-xs text-muted-foreground">{whoMergesText}</p>
  }

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
      {canMerge ? null : (
        <p className="text-xs text-muted-foreground">{authorDiscardText}</p>
      )}
      {hint === null ? null : (
        <p id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </p>
      )}
    </div>
  )
}
