import {
  useCallback,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type Dispatch,
} from "react"
import { CircleAlertIcon } from "lucide-react"

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
import { useSpecValidator } from "@/lib/use-spec-validator"
import type { Dashboard } from "@/spec"

import type { DraftAction, EditablePanel } from "./draft"
import { JsonEditor } from "./json-editor"
import { appliedKey, evaluateJson, issueRows, textForDraft } from "./json-edit"
import { formatPanel, type JsonIssue } from "./panel-json"

export type LeaveGuard = (leave: () => void, stay?: () => void) => void

export type JsonViewProps = {
  spec: Dashboard
  draft: EditablePanel
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
  onValidityChange: (valid: boolean) => void
  onLeaveRequest: (guard: LeaveGuard) => void
}

type PendingLeave = { leave: () => void; stay?: (() => void) | undefined }

const validateDelayMs = 250

export function JsonView({
  spec,
  draft,
  dispatch,
  readOnly,
  onValidityChange,
  onLeaveRequest,
}: JsonViewProps) {
  const validate = useSpecValidator()
  const [text, setText] = useState(() => formatPanel(draft))
  const [issues, setIssues] = useState<JsonIssue[]>([])
  const [asking, setAsking] = useState(false)
  const pending = useRef<PendingLeave | null>(null)
  const applied = useRef(appliedKey(draft))
  const draftRef = useRef(draft)
  const uncheckedEdit = useRef(false)
  const unvalidated = useRef<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const invalidRef = useRef(false)
  const invalid = issues.length > 0

  useEffect(() => {
    draftRef.current = draft
  }, [draft])

  useEffect(() => {
    const next = textForDraft(draft, applied.current, !invalid)
    if (next === null) return
    applied.current = appliedKey(draft)
    setText(next)
  }, [draft, invalid])

  function evaluate(next: string) {
    if (!validate) {
      uncheckedEdit.current = true
      onValidityChange(false)
      return
    }
    const result = evaluateJson(next, spec, draftRef.current, validate)
    if (result.kind === "invalid") {
      invalidRef.current = true
      setIssues(result.issues)
      onValidityChange(false)
      return
    }
    if (result.changesDraft) {
      draftRef.current = result.panel
      dispatch({ type: "replace", panel: result.panel })
    }
    applied.current = appliedKey(result.panel)
    invalidRef.current = false
    setIssues([])
    onValidityChange(true)
  }

  function flush() {
    clearTimeout(timer.current)
    const next = unvalidated.current
    unvalidated.current = null
    if (next !== null) evaluate(next)
  }
  const flushRef = useRef(flush)
  useEffect(() => {
    flushRef.current = flush
  })

  useEffect(
    () => () => {
      flushRef.current()
      onValidityChange(true)
    },
    [onValidityChange]
  )

  const guard = useCallback<LeaveGuard>((leave, stay) => {
    flushRef.current()
    if (!invalidRef.current) {
      leave()
      return
    }
    pending.current = { leave, stay }
    setAsking(true)
  }, [])
  useEffect(() => {
    onLeaveRequest(guard)
  }, [guard, onLeaveRequest])

  function edit(next: string) {
    setText(next)
    if (!validate) {
      evaluate(next)
      return
    }
    unvalidated.current = next
    onValidityChange(false)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => flushRef.current(), validateDelayMs)
  }

  const checkEarlyEdit = useEffectEvent(() => {
    if (!uncheckedEdit.current) return
    uncheckedEdit.current = false
    evaluate(text)
  })
  useEffect(() => {
    if (validate) checkEarlyEdit()
  }, [validate])

  function takePending() {
    const taken = pending.current
    pending.current = null
    setAsking(false)
    return taken
  }

  function keepEditing() {
    takePending()?.stay?.()
  }

  function discard() {
    const taken = takePending()
    if (!taken) return
    applied.current = appliedKey(draftRef.current)
    invalidRef.current = false
    setText(formatPanel(draftRef.current))
    setIssues([])
    onValidityChange(true)
    taken.leave()
  }

  const { rows, more } = issueRows(issues)

  return (
    <div className="flex flex-col gap-1.5">
      <p className="text-xs text-muted-foreground">
        The draft panel as it will be saved in the ObserveSpec. Edits here
        update the editor.
      </p>
      <JsonEditor
        value={text}
        onChange={edit}
        readOnly={readOnly}
        ariaLabel="Panel JSON"
      />
      {invalid ? (
        <ul aria-live="polite" className="flex flex-col gap-1">
          {rows.map((row, index) => (
            <li
              key={index}
              className="flex items-start gap-1.5 text-xs text-status-critical"
            >
              <CircleAlertIcon
                aria-hidden="true"
                className="mt-0.5 size-3.5 shrink-0"
              />
              <span className="min-w-0 break-words">
                {row.location?.kind === "path" ? (
                  <span className="font-mono">{row.location.text}</span>
                ) : (
                  row.location?.text
                )}
                {row.location ? ": " : null}
                {row.message}
              </span>
            </li>
          ))}
          {more > 0 ? (
            <li className="text-xs text-muted-foreground">and {more} more</li>
          ) : null}
        </ul>
      ) : null}
      <AlertDialog
        open={asking}
        onOpenChange={(open) => {
          if (!open) keepEditing()
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard the JSON edits?</AlertDialogTitle>
            <AlertDialogDescription>
              The JSON has errors. The editor keeps the last valid draft.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep editing</AlertDialogCancel>
            <AlertDialogAction onClick={discard}>
              Discard JSON edits
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
