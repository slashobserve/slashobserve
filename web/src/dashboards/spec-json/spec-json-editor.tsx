import { useEffect, useId, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useSpecValidator } from "@/lib/use-spec-validator"
import { JsonEditor } from "@/panel-editor/json-editor"
import { panelTitles } from "@/history/diff-groups"
import type { Dashboard } from "@/spec"

import { MergeConflicts } from "../edit/merge-conflicts"
import { saveProblemFrom, type Problem } from "../edit/save-problem"
import {
  asDashboardSpec,
  latestDashboardQuery,
  useAddVersion,
} from "../queries"
import { saveToast } from "../save-outcome"
import { evaluateSpecJson, formatSpec } from "./spec-json"

const maxNoteLength = 500
const defaultSummary = "Edit the dashboard JSON"
const rejectedHeading = "The server rejected this Spec:"

type Props = {
  dashboardId: string
  spec: Dashboard
  version: number
  onDirtyChange: (dirty: boolean) => void
  onSaved: () => void
}

export default function SpecJsonEditor({
  dashboardId,
  spec,
  version,
  onDirtyChange,
  onSaved,
}: Props) {
  const queryClient = useQueryClient()
  const validate = useSpecValidator()
  const save = useAddVersion(dashboardId)
  const noteId = useId()
  const [base, setBase] = useState({ spec, version })
  const [text, setText] = useState(() => formatSpec(spec))
  const [note, setNote] = useState("")
  const [problem, setProblem] = useState<Problem | null>(null)
  const [reloading, setReloading] = useState(false)
  const result = useMemo(
    () =>
      validate === undefined
        ? null
        : evaluateSpecJson(text, base.spec, validate),
    [text, base.spec, validate]
  )
  const dirty = text !== formatSpec(base.spec)
  const titles = useMemo(() => panelTitles(base.spec), [base.spec])
  const pending = save.isPending || reloading

  useEffect(() => {
    onDirtyChange(dirty)
  }, [dirty, onDirtyChange])

  let summary = "Checking the JSON."
  if (result?.kind === "unchanged") summary = "No changes"
  else if (result?.kind === "invalid") summary = "Fix the JSON to save."
  else if (result?.kind === "valid") summary = "Saves the whole Spec"

  function submit() {
    if (result?.kind !== "valid" || pending) return
    setProblem(null)
    save.mutate(
      {
        spec: result.spec,
        summary: note.trim() || defaultSummary,
        parent_version: base.version,
        propose: false,
      },
      {
        onSuccess: (outcome) => {
          toast.success(saveToast(outcome))
          onSaved()
        },
        onError: (failure) =>
          setProblem(saveProblemFrom(failure, rejectedHeading)),
      }
    )
  }

  async function reload() {
    setReloading(true)
    try {
      const latest = await queryClient.fetchQuery({
        ...latestDashboardQuery(queryClient, dashboardId),
        staleTime: 0,
      })
      const latestSpec = asDashboardSpec(latest.version.spec)
      setBase({ spec: latestSpec, version: latest.version.version })
      setText(formatSpec(latestSpec))
      setProblem(null)
    } catch (failure) {
      setProblem(saveProblemFrom(failure, rejectedHeading))
    } finally {
      setReloading(false)
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-4 py-3.5">
        <p className="text-[13px] text-muted-foreground">
          Variables, links, time settings and every panel live here. A valid
          edit saves as the next version after version {base.version}; a version
          saved meanwhile is merged when it changed other things.
        </p>
        <JsonEditor
          value={text}
          onChange={setText}
          readOnly={pending}
          ariaLabel="Dashboard Spec JSON"
        />
        {result?.kind === "invalid" ? (
          <div role="status" className="text-[13px] text-destructive">
            <ul className="list-disc pl-4 wrap-anywhere">
              {result.lines.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          </div>
        ) : null}
        {problem?.kind === "conflict" ? (
          <MergeConflicts
            conflicts={problem.conflicts}
            titles={titles}
            latest={problem.latest}
            onReload={() => void reload()}
          />
        ) : null}
        {problem?.kind === "invalid" ? (
          <Alert variant="destructive">
            <AlertDescription>
              <p>{problem.heading}</p>
              <ul className="list-disc pl-4 wrap-anywhere">
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
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={noteId} className="text-[13px]">
            Note for the timeline
          </Label>
          <Input
            id={noteId}
            value={note}
            placeholder={defaultSummary}
            maxLength={maxNoteLength}
            disabled={pending}
            onChange={(event) => setNote(event.target.value)}
          />
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-2 border-t px-4 pt-3 pb-3.5">
        <span className="mr-auto text-xs text-muted-foreground">{summary}</span>
        <Button
          variant="outline"
          disabled={pending || !dirty}
          onClick={() => {
            setText(formatSpec(base.spec))
            setProblem(null)
          }}
        >
          Discard
        </Button>
        <Button
          focusableWhenDisabled
          disabled={result?.kind !== "valid" || pending}
          aria-busy={save.isPending}
          onClick={submit}
        >
          {save.isPending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : null}
          Save as version {base.version + 1}
        </Button>
      </div>
    </div>
  )
}
