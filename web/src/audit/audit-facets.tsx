import { useRef, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { MinusIcon } from "lucide-react"

import type { AuditWindowKey } from "@/api/keys"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import { liveFacetValues } from "./live-counts"
import { auditFacetsQuery, auditFieldsQuery, facetValues } from "./queries"
import {
  canEditTerms,
  termState,
  toggleTermRemembering,
  type WrapMemory,
  type FacetField,
} from "./query-text"
import type { AuditEvent } from "./stream"

const facetFields = [
  { name: "actor_name", label: "Actor" },
  { name: "action", label: "Action" },
  { name: "outcome", label: "Outcome" },
  { name: "origin", label: "Origin" },
  { name: "initiator", label: "Initiator" },
] as const

export function AuditFacets({
  window,
  query,
  live,
  onQuery,
}: {
  window: AuditWindowKey
  query: string
  live: readonly AuditEvent[]
  onQuery: (q: string) => void
}) {
  const fields = useQuery(auditFieldsQuery())
  const wrapMemory = useRef<WrapMemory | null>(null)
  const editable = canEditTerms(query)
  return (
    <div className="flex flex-col gap-2">
      {editable ? null : (
        <p className="text-xs text-muted-foreground">
          Filter chips edit KQL only. Change the query text to filter a Lucene
          or Query DSL search.
        </p>
      )}
      <div className="flex flex-wrap gap-x-6 gap-y-3">
        {facetFields.map((facet) => (
          <FacetGroup
            key={facet.name}
            window={window}
            query={query}
            label={facet.label}
            field={{
              name: facet.name,
              alias:
                fields.data?.fields.find((field) => field.name === facet.name)
                  ?.alias ?? null,
            }}
            editable={editable}
            live={live}
            wrapMemoryRef={wrapMemory}
            onQuery={onQuery}
          />
        ))}
      </div>
    </div>
  )
}

function FacetGroup({
  window,
  query,
  label,
  field,
  editable,
  live,
  wrapMemoryRef,
  onQuery,
}: {
  window: AuditWindowKey
  query: string
  label: string
  field: FacetField
  editable: boolean
  live: readonly AuditEvent[]
  wrapMemoryRef: RefObject<WrapMemory | null>
  onQuery: (q: string) => void
}) {
  const facets = useQuery(auditFacetsQuery(window, field.name))
  if (facets.isError) return null
  const values = liveFacetValues(
    facets.data?.values ?? [],
    live,
    field.name,
    facetValues
  )

  function toggle(value: string, exclude: boolean) {
    const next = toggleTermRemembering(
      query,
      field,
      value,
      exclude,
      wrapMemoryRef.current
    )
    if (next === null) return
    wrapMemoryRef.current = next.memory
    onQuery(next.text)
  }

  return (
    <div
      role="group"
      aria-label={`${label} values`}
      className="flex min-w-0 flex-col gap-1.5"
    >
      <span className="text-xs text-muted-foreground">{label}</span>
      {facets.data === undefined ? (
        <Skeleton className="h-6 w-32" />
      ) : values.length === 0 ? (
        <span className="text-xs text-muted-foreground">None</span>
      ) : (
        <ul className="flex flex-wrap gap-1.5">
          {values.map(({ value, count }) => {
            const state = termState(query, field, value)
            return (
              <li
                key={value}
                className={cn(
                  "inline-flex h-6 items-center rounded-chip border text-xs",
                  state === "include" &&
                    "border-primary bg-primary-container text-on-primary-container"
                )}
              >
                <button
                  type="button"
                  aria-pressed={state === "include"}
                  disabled={!editable}
                  onClick={() => toggle(value, false)}
                  className="inline-flex h-full max-w-[220px] items-center gap-1.5 rounded-chip px-2 outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <span
                    className={cn(
                      "truncate",
                      state === "exclude" && "line-through"
                    )}
                  >
                    {value}
                  </span>
                  <span className="text-muted-foreground tabular-nums">
                    {count.toLocaleString("en-US")}
                  </span>
                </button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-pressed={state === "exclude"}
                  aria-label={`Leave out ${label.toLowerCase()} ${value}`}
                  title={`Leave out ${value}`}
                  disabled={!editable}
                  onClick={() => toggle(value, true)}
                >
                  <MinusIcon aria-hidden="true" />
                </Button>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
