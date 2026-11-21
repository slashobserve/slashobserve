import { useQuery } from "@tanstack/react-query"
import { MinusIcon } from "lucide-react"

import type { AuditWindowKey } from "@/api/keys"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import { auditFacetsQuery, auditFieldsQuery } from "./queries"
import {
  canEditTerms,
  termState,
  toggleTerm,
  type FacetField,
} from "./query-text"

const facetFields = [
  { name: "actor_name", label: "Actor" },
  { name: "action", label: "Action" },
  { name: "outcome", label: "Outcome" },
  { name: "origin", label: "Origin" },
  { name: "initiator", label: "Initiator" },
] as const

export function AuditFacets({
  window,
  onQuery,
}: {
  window: AuditWindowKey
  onQuery: (q: string) => void
}) {
  const fields = useQuery(auditFieldsQuery())
  const editable = canEditTerms(window.q)
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
            label={facet.label}
            field={{
              name: facet.name,
              alias:
                fields.data?.fields.find((field) => field.name === facet.name)
                  ?.alias ?? null,
            }}
            editable={editable}
            onQuery={onQuery}
          />
        ))}
      </div>
    </div>
  )
}

function FacetGroup({
  window,
  label,
  field,
  editable,
  onQuery,
}: {
  window: AuditWindowKey
  label: string
  field: FacetField
  editable: boolean
  onQuery: (q: string) => void
}) {
  const facets = useQuery(auditFacetsQuery(window, field.name))
  if (facets.isError) return null

  function toggle(value: string, exclude: boolean) {
    const next = toggleTerm(window.q, field, value, exclude)
    if (next !== null) onQuery(next)
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
      ) : facets.data.values.length === 0 ? (
        <span className="text-xs text-muted-foreground">None</span>
      ) : (
        <ul className="flex flex-wrap gap-1.5">
          {facets.data.values.map(({ value, count }) => {
            const state = termState(window.q, field, value)
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
