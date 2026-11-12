import { useId, useState, type FormEvent } from "react"
import { useQueryClient } from "@tanstack/react-query"

import { observationKeys } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

import { blockWriteProblem } from "./errors"
import { useResolveObservation, type Observation } from "./queries"
import { maxSummaryLength, summaryProblem } from "./summary"

export const stateActionId = "observation-state-action"

type Props = {
  observation: Observation
  open: boolean
  onOpenChange: (open: boolean) => void
}

function focusStateAction() {
  return document.getElementById(stateActionId)
}

function ResolveForm({
  observation,
  onClose,
}: {
  observation: Observation
  onClose: () => void
}) {
  const ids = useId()
  const queryClient = useQueryClient()
  const resolve = useResolveObservation(observation.id)
  const [summary, setSummary] = useState(observation.summary ?? "")
  const [serverProblem, setServerProblem] = useState<string | null>(null)
  const problem = summaryProblem(summary)
  const showProblem = problem !== null && summary !== ""
  const problemId = `${ids}-problem`

  function submit(event: FormEvent) {
    event.preventDefault()
    if (problem !== null || resolve.isPending) return
    setServerProblem(null)
    resolve.mutate(
      { summary: summary.trim() },
      {
        onSuccess: onClose,
        onError: (error) => {
          const failure = blockWriteProblem(error)
          setServerProblem(failure.text)
          if (failure.kind === "refused") {
            void queryClient.invalidateQueries({
              queryKey: observationKeys.one(observation.id),
              exact: true,
            })
          }
        },
      }
    )
  }

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${ids}-summary`}>Summary</Label>
        <Input
          id={`${ids}-summary`}
          value={summary}
          autoComplete="off"
          autoFocus
          placeholder="What happened, in one line"
          readOnly={resolve.isPending}
          aria-invalid={showProblem}
          aria-describedby={showProblem ? problemId : undefined}
          onChange={(event) => setSummary(event.target.value)}
        />
        <div className="flex items-start justify-between gap-3 text-xs">
          <p
            id={problemId}
            className={showProblem ? "text-destructive" : undefined}
          >
            {showProblem ? problem : null}
          </p>
          <span className="flex-none text-muted-foreground">
            {[...summary.trim()].length}/{maxSummaryLength}
          </span>
        </div>
      </div>
      {serverProblem === null ? null : (
        <Alert variant="destructive">
          <AlertDescription>{serverProblem}</AlertDescription>
        </Alert>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onClose}>
          Cancel
        </Button>
        <Button
          type="submit"
          focusableWhenDisabled
          disabled={problem !== null || resolve.isPending}
          aria-busy={resolve.isPending}
        >
          Resolve
        </Button>
      </div>
    </form>
  )
}

export function ResolveDialog({ observation, open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent finalFocus={focusStateAction}>
        <DialogHeader>
          <DialogTitle>Resolve this observation</DialogTitle>
          <DialogDescription>
            The summary shows under the title and in the list.
          </DialogDescription>
        </DialogHeader>
        <ResolveForm
          observation={observation}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  )
}
