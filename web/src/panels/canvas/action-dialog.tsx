import { useRef, useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { canvasActionKeys } from "@/api/keys"
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

import {
  actionOutcome,
  actionProblem,
  requestLine,
  sentFromServerLine,
  type ActionProblem,
  type ActionRequestBody,
  type ElementAction,
} from "./actions"

export type PendingAction = {
  action: ElementAction
  request: ActionRequestBody
}

function useRunCanvasAction(dashboardId: string) {
  return useMutation({
    mutationKey: canvasActionKeys.run(dashboardId),
    networkMode: "always",
    mutationFn: (body: ActionRequestBody) =>
      api
        .POST("/dashboards/{id}/actions", {
          params: { path: { id: dashboardId } },
          body,
        })
        .then(unwrap),
  })
}

export function ActionDialog({
  dashboardId,
  pending,
  onClose,
}: {
  dashboardId: string
  pending: PendingAction
  onClose: () => void
}) {
  const { action, request } = pending
  const run = useRunCanvasAction(dashboardId)
  const [problem, setProblem] = useState<ActionProblem | null>(null)
  const cancel = useRef<HTMLButtonElement>(null)

  function changeOpen(open: boolean) {
    if (!open && !run.isPending) onClose()
  }

  function confirm() {
    setProblem(null)
    run.mutate(request, {
      onSuccess: (result) => {
        const outcome = actionOutcome(action.title, result)
        if (outcome.tone === "success") toast.success(outcome.text)
        else toast.error(outcome.text)
        onClose()
      },
      onError: (error) => setProblem(actionProblem(error, action.host)),
    })
  }

  return (
    <AlertDialog open onOpenChange={changeOpen}>
      <AlertDialogContent initialFocus={cancel}>
        <AlertDialogHeader>
          <AlertDialogTitle className="wrap-anywhere">
            {action.title}
          </AlertDialogTitle>
          <AlertDialogDescription className="wrap-anywhere">
            {action.confirm === null ? null : (
              <span className="mb-2 block text-foreground">
                {action.confirm}
              </span>
            )}
            <span className="block font-medium text-foreground">
              {requestLine(action)}
            </span>
            <span className="block">{sentFromServerLine}</span>
          </AlertDialogDescription>
        </AlertDialogHeader>
        {problem === null ? null : (
          <Alert variant="destructive">
            <AlertDescription>
              {problem.text}
              {problem.allowlist ? (
                <>
                  {" "}
                  <Link
                    to="/settings"
                    hash="outbound-access"
                    className="font-medium underline underline-offset-2"
                    onClick={onClose}
                  >
                    Outbound access
                  </Link>
                </>
              ) : null}
            </AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel ref={cancel} aria-disabled={run.isPending}>
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={run.isPending}
            focusableWhenDisabled
            aria-busy={run.isPending}
            onClick={confirm}
          >
            {run.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Run
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
