import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { ApiError, type ApiIssue } from "@/api/errors"
import { isTransient, issueLines } from "@/api/issues"
import { dashboardKeys } from "@/api/keys"
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
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { useLatestPermissions } from "@/dashboards/permissions"
import { latestDashboardQuery, useRestoreVersion } from "@/dashboards/queries"

type Props = {
  dashboardId: string
  version: number
  latestVersion: number
  onRestored: () => void
}

type Problem =
  | { kind: "conflict"; latest: number }
  | { kind: "invalid"; issues: ApiIssue[] }
  | { kind: "failed"; message: string }

function problemFrom(error: unknown): Problem {
  if (error instanceof ApiError) {
    if (error.status === 409 && error.latestVersion !== undefined) {
      return { kind: "conflict", latest: error.latestVersion }
    }
    if (error.status === 422) return { kind: "invalid", issues: error.issues }
    return { kind: "failed", message: error.message }
  }
  return {
    kind: "failed",
    message: error instanceof Error ? error.message : "The restore failed.",
  }
}

export function RestoreVersionButton({
  dashboardId,
  version,
  latestVersion,
  onRestored,
}: Props) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [openedAt, setOpenedAt] = useState(latestVersion)
  const [knownLatest, setKnownLatest] = useState(0)
  const queryClient = useQueryClient()
  const restore = useRestoreVersion(dashboardId)
  const canEdit = useLatestPermissions(dashboardId).edit
  const parent = Math.max(openedAt, knownLatest)
  const conflicted = knownLatest > 0

  function openDialog() {
    setOpenedAt(latestVersion)
    setOpen(true)
  }

  function changeOpen(next: boolean) {
    // Cancel is only aria-disabled while pending (Close has no focusableWhenDisabled), so closing is blocked here
    if (!next && restore.isPending) return
    setOpen(next)
    if (!next) {
      setProblem(null)
      setKnownLatest(0)
      restore.reset()
    }
  }

  function confirm() {
    setProblem(null)
    restore.mutate(
      { version, parent_version: parent },
      {
        onSuccess: (saved) => {
          toast.success(
            `Restored version ${version} as version ${saved.version.version}`
          )
          setOpen(false)
          onRestored()
        },
        onError: (error) => {
          const next = problemFrom(error)
          setProblem(next)
          if (next.kind !== "conflict") return
          setKnownLatest(next.latest)
          void queryClient
            .fetchQuery({
              ...latestDashboardQuery(queryClient, dashboardId),
              staleTime: 0,
            })
            .catch(() => undefined)
          void queryClient.invalidateQueries({
            queryKey: dashboardKeys.history(dashboardId),
          })
        },
      }
    )
  }

  if (!canEdit) return null

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        aria-label={`Restore version ${version}`}
        onClick={openDialog}
      >
        Restore
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Restore version {version}?</AlertDialogTitle>
            <AlertDialogDescription>
              This saves a new version {parent + 1} with the dashboard exactly
              as it was in version {version}. Nothing is deleted.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem?.kind === "conflict" ? (
            <Alert>
              <AlertDescription className="text-foreground">
                Version {problem.latest} was saved meanwhile.
              </AlertDescription>
            </Alert>
          ) : null}
          {problem?.kind === "invalid" ? (
            <Alert variant="destructive">
              <AlertDescription>
                <p>
                  {problem.issues.length > 0 &&
                  problem.issues.every(isTransient)
                    ? "This version could not be restored right now."
                    : "This version cannot be restored."}
                </p>
                <ul className="list-disc pl-4">
                  {issueLines(problem.issues).map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          ) : null}
          {problem?.kind === "failed" ? (
            <Alert variant="destructive">
              <AlertDescription>{problem.message}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={restore.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={restore.isPending}
              focusableWhenDisabled
              aria-busy={restore.isPending}
              onClick={confirm}
            >
              {restore.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              {conflicted ? `Restore on top of version ${parent}` : "Restore"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
