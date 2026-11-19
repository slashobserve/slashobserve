import { useId, useState, type FormEvent } from "react"

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

import { deleteFailureText } from "./delete-dialog"
import { useResolveObservations } from "./queries"
import { maxSummaryLength, summaryProblem } from "./summary"

type ResolveTarget = { id: string; title: string }

type Props = {
  targets: readonly ResolveTarget[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onResolved: (ids: readonly string[]) => void
}

function ResolveManyForm({
  targets,
  onClose,
  onResolved,
}: Omit<Props, "open" | "onOpenChange"> & { onClose: () => void }) {
  const ids = useId()
  const resolve = useResolveObservations()
  const [summary, setSummary] = useState("")
  const [pending, setPending] = useState(false)
  const [serverProblem, setServerProblem] = useState<string | null>(null)
  const problem = summaryProblem(summary)
  const showProblem = problem !== null && summary !== ""
  const problemId = `${ids}-problem`

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (problem !== null || pending) return
    setPending(true)
    setServerProblem(null)
    const done: string[] = []
    let failure: unknown = null
    for (const target of targets) {
      try {
        await resolve.mutateAsync({ id: target.id, summary: summary.trim() })
        done.push(target.id)
      } catch (error) {
        failure = error
      }
    }
    setPending(false)
    if (done.length > 0) onResolved(done)
    if (failure === null) {
      onClose()
      return
    }
    const left = targets.length - done.length
    setServerProblem(
      `${left === 1 ? "One observation" : `${left} observations`} could not be resolved: ${deleteFailureText(failure)}`
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
          readOnly={pending}
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
          disabled={problem !== null || pending}
          aria-busy={pending}
        >
          {targets.length === 1 ? "Resolve" : `Resolve ${targets.length}`}
        </Button>
      </div>
    </form>
  )
}

export function ResolveManyDialog({
  targets,
  open,
  onOpenChange,
  onResolved,
}: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {targets.length === 1
              ? "Resolve this observation"
              : `Resolve ${targets.length} observations`}
          </DialogTitle>
          <DialogDescription>
            {targets.length === 1
              ? "The summary shows under the title and in the list."
              : `This summary applies to all ${targets.length} and shows under each title and in the list.`}
          </DialogDescription>
        </DialogHeader>
        <ResolveManyForm
          targets={targets}
          onResolved={onResolved}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  )
}
