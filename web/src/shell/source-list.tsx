import { Link } from "@tanstack/react-router"

import { SourceLink } from "@/datasources/source-menu"
import type { DatasourceStatus } from "@/datasources/queries"
import type { ExploreKind } from "@/datasources/sources"
import { cn } from "@/lib/utils"

import {
  SubNavList,
  SubNavSkeleton,
  subNavLinkClass,
  subNavMoreClass,
  useReservedRows,
} from "./sub-nav"

export function SourceList({
  kind,
  shown,
  more,
  pending = false,
  currentId,
  onNavigate,
}: {
  kind: ExploreKind
  shown: readonly DatasourceStatus[]
  more: boolean
  pending?: boolean
  currentId: string | null
  onNavigate?: () => void
}) {
  const label = kind === "logs" ? "Log sources" : "Trace sources"
  const reserved = useReservedRows(kind, pending, shown.length + (more ? 1 : 0))
  if (pending) return <SubNavSkeleton label={label} rows={reserved} />
  if (shown.length === 0) return null
  const word = kind === "logs" ? "log" : "trace"

  return (
    <SubNavList label={label}>
      {shown.map((source) => (
        <li key={source.uid}>
          <SourceLink
            kind={kind}
            source={source}
            current={source.uid === currentId}
            onNavigate={onNavigate}
            className={cn(subNavLinkClass, "justify-between")}
          />
        </li>
      ))}
      {more ? (
        <li>
          <Link
            to={kind === "logs" ? "/logs" : "/traces"}
            onClick={onNavigate}
            aria-label={`More ${word} sources`}
            className={subNavMoreClass}
          >
            More
          </Link>
        </li>
      ) : null}
    </SubNavList>
  )
}
