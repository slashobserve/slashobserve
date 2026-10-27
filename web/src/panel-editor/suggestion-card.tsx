import { useQuery } from "@tanstack/react-query"

import { StatusIcon } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { sameJson } from "@/lib/json"
import { queryRef, type PanelQuery } from "@/spec"

import { changeSummary, panelChanges } from "./changes"
import type { EditablePanel } from "./draft"
import { lineDiff, type DiffLine } from "./line-diff"
import { describeSuggestionError, suggestionQuery } from "./queries"

const maxValueCodePoints = 80

type Fields = Record<string, unknown>

type QueryDiff = { ref: string; lines: DiffLine[] }

type PanelDiff = { queries: QueryDiff[]; fields: string[] }

const unusablePanelMessage = "The agent did not return a usable panel."

const linePrefixes: Record<DiffLine["kind"], string> = {
  same: " ",
  add: "+",
  del: "-",
}

const lineStyles: Record<DiffLine["kind"], string> = {
  same: "",
  add: "bg-diff-add text-diff-add-foreground",
  del: "bg-diff-del text-diff-del-foreground",
}

function shortJson(value: unknown): string {
  const text = value === undefined ? "none" : JSON.stringify(value)
  const points = Array.from(text)
  return points.length > maxValueCodePoints
    ? `${points.slice(0, maxValueCodePoints).join("")}…`
    : text
}

function queriesOf(panel: Fields): PanelQuery[] {
  return Array.isArray(panel.queries) ? (panel.queries as PanelQuery[]) : []
}

function byRef(queries: PanelQuery[]): Map<string, PanelQuery> {
  return new Map(queries.map((query, index) => [queryRef(query, index), query]))
}

function panelDiff(started: EditablePanel, proposed: EditablePanel): PanelDiff {
  const before: Fields = { ...started }
  const after: Fields = { ...proposed }
  const beforeQueries = byRef(queriesOf(before))
  const afterQueries = byRef(queriesOf(after))
  const refs = [...new Set([...afterQueries.keys(), ...beforeQueries.keys()])]

  const queries: QueryDiff[] = []
  const fields: string[] = []
  for (const ref of refs) {
    const was = beforeQueries.get(ref)
    const now = afterQueries.get(ref)
    if (was?.query !== now?.query) {
      queries.push({ ref, lines: lineDiff(was?.query ?? "", now?.query ?? "") })
    }
    if (was === undefined || now === undefined) continue
    const wasFields: Fields = { ...was }
    const nowFields: Fields = { ...now }
    for (const key of Object.keys({ ...wasFields, ...nowFields }).sort()) {
      if (key === "query" || key === "ref") continue
      if (!sameJson(wasFields[key], nowFields[key])) {
        fields.push(
          `Query ${ref} ${key}: ${shortJson(wasFields[key])} → ${shortJson(nowFields[key])}`
        )
      }
    }
  }

  for (const key of Object.keys({ ...before, ...after }).sort()) {
    if (key === "queries") continue
    if (!sameJson(before[key], after[key])) {
      fields.push(
        `${key}: ${shortJson(before[key])} → ${shortJson(after[key])}`
      )
    }
  }
  return { queries, fields }
}

type Props = {
  dashboardId: string
  suggestionId: string
  started: EditablePanel
  onAccept: (proposed: EditablePanel) => void
  onDiscard: () => void
  onRetry: () => void
}

const cardClass =
  "flex flex-col gap-2 rounded-lg border border-dashed p-2.5 text-[13px]"

export function SuggestionCard({
  dashboardId,
  suggestionId,
  started,
  onAccept,
  onDiscard,
  onRetry,
}: Props) {
  const found = useQuery(suggestionQuery(dashboardId, suggestionId))
  const suggestion = found.data

  if (suggestion === undefined) {
    if (!found.isError) {
      return (
        <section aria-label="Agent suggestion" className={cardClass}>
          <p className="flex items-center gap-2" role="status">
            <StatusIcon tone="running" />
            Working
          </p>
        </section>
      )
    }
    return (
      <section aria-label="Agent suggestion" className={cardClass}>
        <p role="alert" className="flex items-start gap-2">
          <StatusIcon tone="critical" />
          {describeSuggestionError(found.error)}
        </p>
        <div className="flex gap-2">
          <Button size="xs" variant="outline" onClick={onDiscard}>
            Discard
          </Button>
        </div>
      </section>
    )
  }

  const settled =
    suggestion.status === "succeeded" || suggestion.status === "failed"
  const proposed = suggestion.panel as EditablePanel | null
  const usable =
    suggestion.status === "succeeded" &&
    proposed !== null &&
    proposed.id === started.id

  if (usable) {
    const changes = panelChanges(started, proposed)
    const diff = panelDiff(started, proposed)
    return (
      <section aria-label="Agent suggestion" className={cardClass}>
        <h3 className="font-semibold">Proposed change</h3>
        {changes.length === 0 ? (
          <p className="text-muted-foreground">The agent proposed no change.</p>
        ) : (
          <>
            <p className="text-muted-foreground">{changeSummary(changes)}</p>
            {diff.queries.map(({ ref, lines }) => (
              <div key={ref} className="flex flex-col gap-1">
                <p className="text-xs text-muted-foreground">Query {ref}</p>
                <div className="overflow-x-auto rounded-control border font-mono text-xs">
                  {lines.map((line, index) => (
                    <div
                      key={index}
                      className={cn(
                        "px-2 py-px break-all whitespace-pre-wrap",
                        lineStyles[line.kind]
                      )}
                    >
                      {linePrefixes[line.kind]} {line.text}
                    </div>
                  ))}
                </div>
              </div>
            ))}
            {diff.fields.map((field) => (
              <p key={field} className="font-mono text-xs break-all">
                {field}
              </p>
            ))}
          </>
        )}
        <div className="flex gap-2">
          {changes.length === 0 ? null : (
            <Button size="xs" onClick={() => onAccept(proposed)}>
              Accept
            </Button>
          )}
          <Button size="xs" variant="outline" onClick={onDiscard}>
            Discard
          </Button>
        </div>
      </section>
    )
  }

  return (
    <section aria-label="Agent suggestion" className={cardClass}>
      <ol
        aria-label="Progress"
        aria-live="polite"
        aria-relevant="additions"
        className="flex flex-col gap-1.5 empty:hidden"
      >
        {suggestion.steps.map((step, index) => (
          <li key={index} className="flex items-start gap-2">
            <span className="flex h-[1.45em] items-center">
              <StatusIcon tone={step.status === "error" ? "critical" : "ok"} />
              {step.status === "error" ? (
                <span className="sr-only">Step failed</span>
              ) : null}
            </span>
            <span className="min-w-0 wrap-anywhere">{step.message}</span>
          </li>
        ))}
        {settled ? null : (
          <li className="flex items-center gap-2">
            <StatusIcon tone="running" />
            Working
          </li>
        )}
      </ol>
      {suggestion.status === "failed" ? (
        <>
          <p
            role="alert"
            className="flex items-start gap-2 text-status-critical"
          >
            <StatusIcon tone="critical" />
            {suggestion.error ?? "The suggestion failed."}
          </p>
          <div className="flex gap-2">
            <Button size="xs" variant="outline" onClick={onRetry}>
              Try again
            </Button>
            <Button size="xs" variant="outline" onClick={onDiscard}>
              Discard
            </Button>
          </div>
        </>
      ) : null}
      {suggestion.status === "succeeded" ? (
        <>
          <p role="alert">{unusablePanelMessage}</p>
          <div className="flex gap-2">
            <Button size="xs" variant="outline" onClick={onDiscard}>
              Discard
            </Button>
          </div>
        </>
      ) : null}
    </section>
  )
}
