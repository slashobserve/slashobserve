import { useId, useImperativeHandle, useRef, useState, type Ref } from "react"
import {
  CheckIcon,
  CircleAlertIcon,
  TriangleAlertIcon,
  XIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { cn } from "@/lib/utils"
import { CodeEditor, type CodeEditorHandle } from "@/query-editor/code-editor"
import { lineAndColumn } from "@/query-editor/line-column"
import type { LintResult } from "@/query-editor/server-lint"
import type { PanelQuery } from "@/spec"

import { queryChanged } from "./changes"
import { DatasourceSelect } from "./datasource-select"

export const maxQueryLength = 20000
const maxLegendLength = 200
const validText = "Query is valid. Errors show under the line that has them."

function hasCoarsePointer() {
  return window.matchMedia("(pointer: coarse)").matches
}

type Props = {
  autoFocus: boolean
  refName: string
  query: PanelQuery
  baseQuery: PanelQuery | undefined
  panelDatasource: string
  readOnly: boolean
  issues: readonly string[]
  onChange: (query: string) => void
  onLegendChange: (legend: string) => void
  onDatasourceChange: (uid: string | null) => void
  onRemove: () => void
  canRemove: boolean
  onFocus: () => void
  editorRef: Ref<CodeEditorHandle>
}

function Diagnostics({
  lint,
  query,
  onPick,
}: {
  lint: LintResult | null
  query: string
  onPick: (from: number) => void
}) {
  const current = lint?.query === query ? lint : null
  if (current?.status === "unavailable") {
    return <p className="text-muted-foreground">{current.message}</p>
  }
  if (current?.status !== "ok") return null
  if (current.items.length > 0) {
    return (
      <ul className="flex flex-col gap-0.5">
        {current.items.map((item, index) => {
          const { line, column } = lineAndColumn(current.query, item.from)
          const isError = item.severity === "error"
          const Icon = isError ? CircleAlertIcon : TriangleAlertIcon
          return (
            <li key={index}>
              <Button
                variant="ghost"
                size="sm"
                className="h-auto w-full items-start justify-start px-1.5 py-1 text-left text-xs whitespace-normal"
                onClick={() => onPick(item.from)}
              >
                <Icon
                  aria-hidden="true"
                  className={cn(
                    "mt-0.5",
                    isError ? "text-status-critical" : "text-status-warning"
                  )}
                />
                <span>
                  {isError ? "Error" : "Warning"}: {item.message} (line {line},
                  column {column})
                </span>
              </Button>
            </li>
          )
        })}
      </ul>
    )
  }
  if (query.trim() === "") return null
  return (
    <p className="flex items-center gap-1.5 text-muted-foreground">
      <CheckIcon
        aria-hidden="true"
        className="size-[13px] text-status-ok"
        strokeWidth={2.4}
      />
      {validText}
    </p>
  )
}

export function QueryRow({
  autoFocus,
  refName,
  query,
  baseQuery,
  panelDatasource,
  readOnly,
  issues,
  onChange,
  onLegendChange,
  onDatasourceChange,
  onRemove,
  canRemove,
  onFocus,
  editorRef,
}: Props) {
  const legendId = useId()
  const datasources = useDatasources()
  const editor = useRef<CodeEditorHandle>(null)
  useImperativeHandle(
    editorRef,
    () => ({
      focusAt: (from) => editor.current?.focusAt(from),
      insertAtCursor: (text) => editor.current?.insertAtCursor(text),
    }),
    []
  )
  const [lint, setLint] = useState<LintResult | null>(null)
  const uid = query.datasource ?? panelDatasource
  const marker =
    baseQuery === undefined
      ? "New"
      : queryChanged(baseQuery, query)
        ? "Changed"
        : null
  const tooLong = [...query.query].length > maxQueryLength

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex min-h-6 flex-wrap items-center gap-x-2 gap-y-1 text-[13px]">
        <span className="font-medium">Query {refName}</span>
        <DatasourceSelect
          compact
          value={query.datasource ?? ""}
          includePanelDefault={panelDatasource}
          ariaLabel={`Datasource for query ${refName}`}
          disabled={readOnly}
          onChange={(next) => onDatasourceChange(next === "" ? null : next)}
        />
        <span className="ml-auto flex items-center gap-1">
          {marker ? (
            <span className="text-xs text-muted-foreground">{marker}</span>
          ) : null}
          {canRemove ? (
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={`Remove query ${refName}`}
              onClick={onRemove}
            >
              <XIcon aria-hidden="true" />
            </Button>
          ) : null}
        </span>
      </div>
      {datasources.isPending ? (
        <Skeleton className="h-24 w-full animate-none motion-safe:animate-pulse" />
      ) : (
        <CodeEditor
          value={query.query}
          onChange={onChange}
          datasource={{
            uid,
            type: datasourceTypeOf(datasources.data, uid),
          }}
          readOnly={readOnly}
          ariaLabel={`Query ${refName}`}
          onLint={setLint}
          onFocus={onFocus}
          editorRef={editor}
          autoFocus={autoFocus && !readOnly && !hasCoarsePointer()}
        />
      )}
      <div role="status" className="flex flex-col text-xs">
        <Diagnostics
          lint={lint}
          query={query.query}
          onPick={(from) => editor.current?.focusAt(from)}
        />
      </div>
      {tooLong ? (
        <p className="text-xs text-status-critical">
          The query is longer than {maxQueryLength} characters.
        </p>
      ) : null}
      {issues.map((line, position) => (
        <p key={position} role="alert" className="text-xs text-status-critical">
          {line}
        </p>
      ))}
      <div className="mt-1 flex flex-col gap-1.5">
        <Label htmlFor={legendId} className="text-[13px]">
          Legend
        </Label>
        <Input
          id={legendId}
          value={query.legend ?? ""}
          maxLength={maxLegendLength}
          readOnly={readOnly}
          className={cn("font-mono", readOnly && "text-muted-foreground")}
          onChange={(event) => onLegendChange(event.target.value)}
        />
      </div>
    </div>
  )
}
