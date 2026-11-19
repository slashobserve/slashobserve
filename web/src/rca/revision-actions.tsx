import { useState } from "react"
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

import { useWithdrawRca } from "./queries"

type Props = {
  observationId: string
  revision: number
  canWithdraw: boolean
  onWithdrawn: () => void
}

export function withdrawProblemText(error: unknown): string {
  if (error instanceof ApiError && error.code === "rca_shared") {
    return `${sentence(error.message)} Revoke its public links below, then withdraw it.`
  }
  if (error instanceof ApiError) return sentence(error.message)
  return "The revision could not be withdrawn. Try again."
}

export function RevisionActions({
  observationId,
  revision,
  canWithdraw,
  onWithdrawn,
}: Props) {
  const [confirming, setConfirming] = useState(false)
  const withdraw = useWithdrawRca(observationId)

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
    <div className="flex flex-wrap gap-2">
      <Link
        to="/o/$observationId/rca/$revision"
        params={{ observationId, revision: String(revision) }}
        className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
      >
        Open revision {revision}
      </Link>
      {canWithdraw ? (
        <Button
          variant="outline"
          size="sm"
          disabled={withdraw.isPending}
          onClick={() => setConfirming(true)}
        >
          Withdraw
        </Button>
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
