import { useMemo, type KeyboardEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { SearchIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { isModEnter, modChordLabel } from "@/lib/shortcuts"
import { formLabels, formOf } from "@/logs/query-form"
import { CodeEditor } from "@/query-editor/code-editor"

import { auditFieldsQuery } from "./queries"
import type { MarkedQuery } from "./query-problem"

export type QueryProblemView = {
  message: string
  marked: MarkedQuery | null
}

export function AuditFilterBar({
  draft,
  onDraft,
  onRun,
  problem,
}: {
  draft: string
  onDraft: (next: string) => void
  onRun: () => void
  problem: QueryProblemView | null
}) {
  const fields = useQuery(auditFieldsQuery())
  const names = useMemo(
    () =>
      (fields.data?.fields ?? []).flatMap((field) =>
        field.alias === null ? [field.name] : [field.name, field.alias]
      ),
    [fields.data]
  )
  const form = formLabels[formOf(draft)]
  const runChord = `${modChordLabel(navigator.userAgent, "")}Enter`

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!isModEnter(event) || event.nativeEvent.isComposing) return
    event.preventDefault()
    event.stopPropagation()
    onRun()
  }

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1" onKeyDown={onKeyDown}>
          <CodeEditor
            value={draft}
            onChange={onDraft}
            datasource={{ type: "opensearch" }}
            readOnly={false}
            ariaLabel={`Audit query, ${form}`}
            completionNames={names}
            onSubmit={onRun}
          />
        </div>
        <Button onClick={onRun} title={`Search (${runChord})`}>
          <SearchIcon aria-hidden="true" />
          Search
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        {form}. Start with lucene: for Lucene or with {"{"} for Query DSL. Enter
        searches, Shift+Enter adds a line, and {runChord} searches too.
      </p>
      {problem !== null ? (
        <div role="alert" className="flex flex-col gap-1">
          {problem.marked !== null ? (
            <pre className="m-0 font-mono text-xs break-all whitespace-pre-wrap">
              {problem.marked.before}
              <mark className="rounded-chip bg-status-critical-container text-destructive underline decoration-wavy">
                {problem.marked.marked}
              </mark>
              {problem.marked.after}
            </pre>
          ) : null}
          <p className="text-xs text-destructive">{problem.message}</p>
        </div>
      ) : null}
    </div>
  )
}
