import {
  useEffect,
  useId,
  useRef,
  type Dispatch,
  type ReactNode,
  type RefObject,
} from "react"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import type { CodeEditorHandle } from "@/query-editor/code-editor"
import { cn } from "@/lib/utils"
import { isDataPanel, queryRef } from "@/spec"

import type { DraftAction, EditablePanel } from "./draft"
import { QueryRow } from "./query-row"

export const maxContentLength = 20000
const maxQueries = 26

type Props = {
  draft: EditablePanel
  base: EditablePanel
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
  issuesByQuery: Record<number, string[]>
  onActiveChange: (index: number) => void
  focusFirst: boolean
  editorRefs: RefObject<(CodeEditorHandle | null)[]>
  beforeAdd?: ReactNode
}

const noIssues: readonly string[] = []

function ContentField({
  value,
  readOnly,
  onChange,
}: {
  value: string
  readOnly: boolean
  onChange: (content: string) => void
}) {
  const id = useId()
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-[13px]">
        Content
      </Label>
      <Textarea
        id={id}
        value={value}
        rows={12}
        maxLength={maxContentLength}
        readOnly={readOnly}
        className={cn(
          "field-sizing-fixed",
          readOnly && "text-muted-foreground"
        )}
        onChange={(event) => onChange(event.target.value)}
      />
      <p className="text-xs text-muted-foreground">
        Markdown. HTML is removed.
      </p>
    </div>
  )
}

export function QueryRows({
  draft,
  base,
  dispatch,
  readOnly,
  issuesByQuery,
  onActiveChange,
  focusFirst,
  editorRefs,
  beforeAdd,
}: Props) {
  const count = isDataPanel(draft) ? draft.queries.length : 0
  const focusNew = useRef(false)
  const removedAt = useRef<number | null>(null)
  const addButton = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (focusNew.current) {
      focusNew.current = false
      editorRefs.current[count - 1]?.focusAt(0)
    }
    const removed = removedAt.current
    if (removed === null) return
    removedAt.current = null
    if (removed > 0) editorRefs.current[removed - 1]?.focusAt(0)
    else addButton.current?.focus()
  }, [count, editorRefs])

  if (!isDataPanel(draft)) {
    return (
      <ContentField
        value={draft.type === "text" ? draft.content : ""}
        readOnly={readOnly}
        onChange={(content) => dispatch({ type: "setContent", content })}
      />
    )
  }

  const baseQueries = isDataPanel(base) ? base.queries : []
  const baseByRef = new Map(
    baseQueries.map((query, index) => [queryRef(query, index), query])
  )

  return (
    <>
      {draft.queries.map((query, index) => {
        const ref = queryRef(query, index)
        return (
          <QueryRow
            key={ref}
            autoFocus={focusFirst && index === 0}
            refName={ref}
            query={query}
            baseQuery={baseByRef.get(ref)}
            panelDatasource={draft.datasource}
            readOnly={readOnly}
            issues={issuesByQuery[index] ?? noIssues}
            canRemove={!readOnly && draft.queries.length > 1}
            onChange={(text) =>
              dispatch({ type: "setQuery", index, query: text })
            }
            onLegendChange={(legend) =>
              dispatch({ type: "setLegend", index, legend })
            }
            onDatasourceChange={(uid) =>
              dispatch({ type: "setQueryDatasource", index, uid })
            }
            onRemove={() => {
              removedAt.current = index
              dispatch({ type: "removeQuery", index })
            }}
            onFocus={() => onActiveChange(index)}
            editorRef={(handle) => {
              editorRefs.current[index] = handle
            }}
          />
        )
      })}
      {beforeAdd}
      <Button
        ref={addButton}
        variant="ghost"
        size="sm"
        className="self-start px-0 text-muted-foreground hover:bg-transparent dark:hover:bg-transparent"
        disabled={readOnly || draft.queries.length >= maxQueries}
        onClick={() => {
          focusNew.current = true
          dispatch({ type: "addQuery" })
        }}
      >
        <PlusIcon aria-hidden="true" className="size-[13px]" />
        Add query
      </Button>
    </>
  )
}
