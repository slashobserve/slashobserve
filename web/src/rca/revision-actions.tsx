import { useId, useState } from "react"
import { Link } from "@tanstack/react-router"
import { toast } from "sonner"

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
import { ApiError } from "@/api/errors"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"

import { useWithdrawRca, type RcaSummary } from "./queries"

type Props = {
  observationId: string
  rca: RcaSummary
  canPublish: boolean
  onWithdrawn: () => void
}

export const sharedWithdrawText =
  "Revoke its public links below to withdraw it."

export function withdrawProblemText(error: unknown): string {
  if (error instanceof ApiError && error.code === "rca_shared") {
    return `${sentence(error.message)} Revoke its public links below, then withdraw it.`
  }
  if (error instanceof ApiError) return sentence(error.message)
  return "The revision could not be withdrawn. Try again."
}

export function RevisionActions({
  observationId,
  rca,
  canPublish,
  onWithdrawn,
}: Props) {
  const hintId = useId()
  const [confirming, setConfirming] = useState(false)
  const withdraw = useWithdrawRca(observationId)
  const { revision } = rca
  const canWithdraw = rca.allowed_actions.includes("withdraw")
  const heldByLinks = !canWithdraw && canPublish && rca.state === "shared"

  function confirm() {
    withdraw.mutate(revision, {
      onSuccess: () => {
        toast.success(`Withdrew revision ${revision}`)
        onWithdrawn()
      },
      onError: (error) => toast.error(withdrawProblemText(error)),
    })
  }

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-wrap gap-2">
        <Link
          to="/o/$observationId/rca/$revision"
          params={{ observationId, revision: String(revision) }}
          className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
        >
          Open revision {revision}
        </Link>
        {canWithdraw || heldByLinks ? (
          <Button
            variant="outline"
            size="sm"
            disabled={heldByLinks || withdraw.isPending}
            focusableWhenDisabled
            aria-describedby={heldByLinks ? hintId : undefined}
            onClick={() => setConfirming(true)}
          >
            Withdraw
          </Button>
        ) : null}
      </div>
      {heldByLinks ? (
        <p id={hintId} className="text-xs text-muted-foreground">
          {sharedWithdrawText}
        </p>
      ) : null}
      <AlertDialog open={confirming} onOpenChange={setConfirming}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Withdraw revision {revision}?</AlertDialogTitle>
            <AlertDialogDescription>
              This deletes the revision and its frozen panels for everyone, and
              it cannot be brought back. The activity log keeps a line that you
              withdrew it. A revision with an active public link cannot be
              withdrawn until the link is revoked.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirm}>
              Withdraw revision
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
