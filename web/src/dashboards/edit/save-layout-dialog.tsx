import { useEffect, useMemo, useRef, useState, type FormEvent } from "react"
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
import { dashboardKeys } from "@/api/keys"
import { panelTitles } from "@/history/diff-groups"
import { loadSpecValidator } from "@/spec"

import { useAddVersion } from "../queries"
import { saveToast } from "../save-outcome"
import { useDashboardSearch, useSetDashboardSearch } from "../search"
import { applyLayout } from "./layout"
import { focusAfterLayoutChange, useLayoutStore } from "./layout-store"
import { MergeConflicts } from "./merge-conflicts"
import { saveProblemFrom, type Problem } from "./save-problem"

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
  const baseSpec = useLayoutStore((state) => state.session?.baseSpec)
  const titles = useMemo(() => panelTitles(baseSpec), [baseSpec])
  const save = useAddVersion(dashboardId)
  const inputRef = useRef<HTMLInputElement>(null)
  const reloadRef = useRef<HTMLButtonElement>(null)
  const [summary, setSummary] = useState(defaultSummary)
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [preparing, setPreparing] = useState(false)
  const pending = save.isPending || preparing
  const error = summaryError(summary)
  const showError = error !== null && (submitted || summary.length > 0)

  const conflicted = problem?.kind === "conflict"
  useEffect(() => {
    if (conflicted) reloadRef.current?.focus()
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

  function finish(message: string) {
    toast.success(message)
    onOpenChange(false)
    end()
    if (search.version !== undefined) {
      setSearch((state) => ({ ...state, version: undefined }))
    }
    focusAfterLayoutChange()
  }

  async function send() {
    const session = useLayoutStore.getState().session
    if (!session) return
    setPreparing(true)
    let validate
    try {
      validate = await loadSpecValidator()
    } catch (failure) {
      setProblem(saveProblemFrom(failure, rejectedHeading))
      return
    } finally {
      setPreparing(false)
    }
    const spec = applyLayout(session.baseSpec, session.layout)
    const checked = validate(spec)
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
        propose: false,
      },
      {
        onSuccess: (outcome) => finish(saveToast(outcome)),
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
    void send()
  }

  function discard() {
    end()
    onOpenChange(false)
    focusAfterLayoutChange()
  }

  function reload() {
    void queryClient.invalidateQueries({
      queryKey: dashboardKeys.latest(dashboardId),
    })
    discard()
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="sm:max-w-md" initialFocus={inputRef}>
        {problem?.kind === "conflict" ? (
          <>
            <DialogHeader>
              <DialogTitle>Save layout</DialogTitle>
              <DialogDescription>Your layout was not saved.</DialogDescription>
            </DialogHeader>
            <MergeConflicts
              conflicts={problem.conflicts}
              titles={titles}
              latest={problem.latest}
              onReload={reload}
              reloadRef={reloadRef}
            />
            <DialogFooter>
              <Button variant="outline" onClick={() => changeOpen(false)}>
                Cancel
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
