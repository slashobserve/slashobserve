import { useId, useState } from "react"
import { Link } from "@tanstack/react-router"

import { cn } from "@/lib/utils"

import { exploreLink } from "./sources"
import { serviceSearch, splitServices, type ServicesState } from "./services"

const linkClass =
  "rounded-sm underline-offset-3 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"

export function SourceServices({
  kind,
  uid,
  state,
}: {
  kind: "logs" | "traces"
  uid: string
  state: ServicesState | undefined
}) {
  const [expanded, setExpanded] = useState(false)
  const listId = useId()
  if (state === undefined || state.status === "loading") {
    return (
      <span className="text-muted-foreground" aria-busy="true">
        Loading services
      </span>
    )
  }
  if (state.status === "unavailable") {
    return <span className="text-muted-foreground">Services unavailable</span>
  }
  if (state.names.length === 0) {
    return <span className="text-muted-foreground">No services seen</span>
  }
  const { shown, hidden } = splitServices(state.names, expanded)
  return (
    <ul
      id={listId}
      aria-label="Services"
      className="flex flex-wrap items-center gap-x-2 gap-y-1"
    >
      {shown.map((name) => (
        <li key={name} className="wrap-anywhere">
          <Link
            {...exploreLink(kind, uid, serviceSearch(kind, state.field, name))}
            className={linkClass}
          >
            {name}
          </Link>
        </li>
      ))}
      {hidden > 0 || expanded ? (
        <li>
          <button
            type="button"
            aria-expanded={expanded}
            aria-controls={listId}
            onClick={() => setExpanded(!expanded)}
            className={cn(linkClass, "text-muted-foreground")}
          >
            {expanded ? "Show fewer" : `+${hidden} more`}
          </button>
        </li>
      ) : null}
      {state.truncated && expanded ? (
        <li className="text-muted-foreground">More exist than are listed</li>
      ) : null}
    </ul>
  )
}
