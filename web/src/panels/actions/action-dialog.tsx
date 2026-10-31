import { useRef, useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { actionKeys } from "@/api/keys"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

import {
  actionOutcome,
  actionProblem,
  maxInputLength,
  requestLine,
  sentFromServerLine,
  withInputs,
  type ActionProblem,
  type ActionRequestBody,
  type PendingRequest,
  type RunnableAction,
} from "./actions"

export type PendingAction = {
  action: RunnableAction
  request: PendingRequest
}

function useRunAction(dashboardId: string) {
  return useMutation({
    mutationKey: actionKeys.run(dashboardId),
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
  const run = useRunAction(dashboardId)
  const [problem, setProblem] = useState<ActionProblem | null>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  const cancel = useRef<HTMLButtonElement>(null)
  const firstInput = useRef<HTMLInputElement>(null)
  const hasInputs = action.inputs.length > 0

  function changeOpen(open: boolean) {
    if (!open && !run.isPending) onClose()
  }

  function confirm() {
    if (run.isPending) return
    setProblem(null)
    run.mutate(withInputs(request, action, values), {
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
      <AlertDialogContent initialFocus={hasInputs ? firstInput : cancel}>
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
        {hasInputs ? (
          <div className="grid gap-3">
            {action.inputs.map((input, position) => (
              <div key={input.key} className="grid gap-1.5">
                <Label htmlFor={`action-input-${input.key}`}>
                  {input.name}
                </Label>
                <Input
                  id={`action-input-${input.key}`}
                  ref={position === 0 ? firstInput : undefined}
                  value={values[input.key] ?? ""}
                  maxLength={maxInputLength}
                  autoComplete="off"
                  readOnly={run.isPending}
                  onChange={(event) =>
                    setValues({ ...values, [input.key]: event.target.value })
                  }
                  onKeyDown={(event) => {
                    if (event.key !== "Enter" || event.nativeEvent.isComposing)
                      return
                    event.preventDefault()
                    confirm()
                  }}
                />
              </div>
            ))}
          </div>
        ) : null}
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
