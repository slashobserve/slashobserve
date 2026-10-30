import { useEffect, useRef, useState, type FormEvent } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { validateSpecSchema } from "@/spec"

import {
  asDashboardSpec,
  latestDashboardQuery,
  useAddVersion,
} from "../queries"
import { useDashboardSearch, useSetDashboardSearch } from "../search"
import { applyLayout } from "./layout"
import { focusEditLayoutButton, useLayoutStore } from "./layout-store"
import { saveProblemFrom, type Problem } from "./save-problem"

const conflictDescriptionId = "layout-conflict-description"
const defaultSummary = "Update layout"
const maxSummaryLength = 500
const rejectedHeading = "The server rejected this layout:"

type Props = {
  dashboardId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

function summaryError(summary: string): string | null {
  if (summary.trim() === "") return "Enter a summary."
  if (summary.length > maxSummaryLength) {
    return `Use at most ${maxSummaryLength} characters.`
  }
  return null
}

export function SaveLayoutDialog({ dashboardId, open, onOpenChange }: Props) {
  const queryClient = useQueryClient()
  const search = useDashboardSearch()
  const setSearch = useSetDashboardSearch()
  const end = useLayoutStore((state) => state.end)
  const save = useAddVersion(dashboardId)
  const inputRef = useRef<HTMLInputElement>(null)
  const reapplyRef = useRef<HTMLButtonElement>(null)
  const [summary, setSummary] = useState(defaultSummary)
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [reapplying, setReapplying] = useState(false)
  const pending = save.isPending || reapplying
  const error = summaryError(summary)
  const showError = error !== null && (submitted || summary.length > 0)

  const conflicted = problem?.kind === "conflict"
  useEffect(() => {
    if (conflicted) reapplyRef.current?.focus()
  }, [conflicted])

  function changeOpen(next: boolean) {
    if (!next && pending) return
    onOpenChange(next)
    if (!next) {
      setProblem(null)
      setSubmitted(false)
      setSummary(defaultSummary)
      save.reset()
    }
  }

  function finish(version: number) {
    toast.success(`Saved version ${version}`)
    onOpenChange(false)
    end()
    if (search.version !== undefined) {
      setSearch((state) => ({ ...state, version: undefined }))
    }
    focusEditLayoutButton()
  }

  function send() {
    const session = useLayoutStore.getState().session
    if (!session) return
    const spec = applyLayout(session.baseSpec, session.layout)
    const checked = validateSpecSchema(spec)
    if (!checked.valid) {
      setProblem({
        kind: "invalid",
        heading: "This layout is not a valid Spec:",
        lines: [
          ...new Set(
            checked.issues.map((issue) => `${issue.path}: ${issue.message}`)
          ),
        ],
      })
      return
    }
    save.mutate(
      {
        spec: checked.spec,
        summary: summary.trim(),
        parent_version: session.baseVersion,
      },
      {
        onSuccess: (saved) => finish(saved.version.version),
        onError: (failure) =>
          setProblem(saveProblemFrom(failure, rejectedHeading)),
      }
    )
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (error !== null || pending) return
    setProblem(null)
    send()
  }

  async function reapply() {
    setReapplying(true)
    try {
      const latest = await queryClient.fetchQuery({
        ...latestDashboardQuery(queryClient, dashboardId),
        staleTime: 0,
      })
      useLayoutStore
        .getState()
        .rebase(latest.version.version, asDashboardSpec(latest.version.spec))
    } catch (failure) {
      setProblem(saveProblemFrom(failure, rejectedHeading))
      setReapplying(false)
      return
    }
    setReapplying(false)
    send()
  }

  function discard() {
    end()
    onOpenChange(false)
    focusEditLayoutButton()
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="sm:max-w-md" initialFocus={inputRef}>
        {problem?.kind === "conflict" ? (
          <>
            <DialogHeader>
              <DialogTitle>Save layout</DialogTitle>
              <DialogDescription id={conflictDescriptionId}>
                Version {problem.latest} was saved while you were editing.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={discard} disabled={pending}>
                Discard my changes
              </Button>
              <Button
                ref={reapplyRef}
                aria-describedby={conflictDescriptionId}
                focusableWhenDisabled
                disabled={pending}
                aria-busy={pending}
                onClick={() => void reapply()}
              >
                {pending ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="size-3.5 motion-safe:animate-spin"
                  />
                ) : null}
                Reapply my layout to version {problem.latest}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <form className="grid gap-4" onSubmit={submit} noValidate>
            <DialogHeader>
              <DialogTitle>Save layout</DialogTitle>
              <DialogDescription>
                This saves your panel positions as a new version.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-2">
              <Label htmlFor="layout-summary">Summary</Label>
              <Input
                id="layout-summary"
                ref={inputRef}
                value={summary}
                readOnly={pending}
                aria-invalid={showError}
                aria-describedby={
                  showError ? "layout-summary-error" : undefined
                }
                onChange={(event) => setSummary(event.target.value)}
              />
              {showError ? (
                <p
                  id="layout-summary-error"
                  role="alert"
                  className="text-sm text-destructive"
                >
                  {error}
                </p>
              ) : null}
            </div>
            {problem?.kind === "invalid" ? (
              <Alert variant="destructive">
                <AlertDescription>
                  <p>{problem.heading}</p>
                  <ul className="list-disc pl-4">
                    {problem.lines.map((line) => (
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
            {pending ? (
              <p className="text-sm text-muted-foreground">
                Checking every panel query before saving.
              </p>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                aria-disabled={pending}
                onClick={() => changeOpen(false)}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                focusableWhenDisabled
                disabled={pending}
                aria-busy={pending}
              >
                {pending ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="size-3.5 motion-safe:animate-spin"
                  />
                ) : null}
                {pending
                  ? "Saving"
                  : problem?.kind === "failed"
                    ? "Try again"
                    : "Save"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
