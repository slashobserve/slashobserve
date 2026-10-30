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
import {
  isDataPanel,
  isDashboardQuery,
  isExpressionQuery,
  isPanelQuery,
  queryRef,
  reusesPanel,
  type DatasourceVariable,
} from "@/spec"

import {
  maxQueries,
  runsAsRange,
  type DraftAction,
  type EditablePanel,
} from "./draft"
import { ExpressionRow } from "./expression-row"
import { SourceRow } from "./source-row"
import { QueryRow, type ResolveDatasource } from "./query-row"

export const maxContentLength = 20000

type Props = {
  draft: EditablePanel
  base: EditablePanel
  dispatch: Dispatch<DraftAction>
  readOnly: boolean
  datasourceVariables: readonly DatasourceVariable[]
  resolveDatasource: ResolveDatasource
  issuesByQuery: ReadonlyMap<string, string[]>
  sourceTitle: (panelId: string) => string
  onActiveChange: (ref: string) => void
  onRemoved: (ref: string) => void
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
        Markdown with HTML. HTML is cleaned: scripts, styles, forms and embeds
        are removed.
      </p>
    </div>
  )
}

export function QueryRows({
  draft,
  base,
  dispatch,
  readOnly,
  datasourceVariables,
  resolveDatasource,
  issuesByQuery,
  sourceTitle,
  onActiveChange,
  onRemoved,
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

  if (draft.type === "dashlist" || draft.type === "news") return null

  if (!isDataPanel(draft)) {
    return (
      <ContentField
        value={draft.content}
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
        const baseQuery = baseByRef.get(ref)
        const remove = () => {
          removedAt.current = index
          onRemoved(ref)
          dispatch({ type: "removeQuery", index })
        }
        if (isExpressionQuery(query)) {
          return (
            <ExpressionRow
              key={ref}
              refName={ref}
              query={query}
              readOnly={readOnly}
              onRemove={remove}
            />
          )
        }
        if (isDashboardQuery(query)) {
          return (
            <SourceRow
              key={ref}
              refName={ref}
              query={query}
              sourceTitle={sourceTitle(query.source.panel)}
            />
          )
        }
        return (
          <QueryRow
            key={ref}
            autoFocus={focusFirst && index === 0}
            refName={ref}
            query={query}
            baseQuery={
              baseQuery && isPanelQuery(baseQuery) ? baseQuery : undefined
            }
            panelDatasource={draft.datasource ?? ""}
            datasourceVariables={datasourceVariables}
            resolveDatasource={resolveDatasource}
            readOnly={readOnly}
            issues={issuesByQuery.get(ref) ?? noIssues}
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
            exemplarsOffered={runsAsRange(query, draft.type)}
            onExemplarChange={(exemplar) =>
              dispatch({ type: "setExemplar", index, exemplar })
            }
            onRemove={remove}
            onFocus={() => onActiveChange(ref)}
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
        disabled={
          readOnly || reusesPanel(draft) || draft.queries.length >= maxQueries
        }
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
