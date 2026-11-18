import { Link } from "@tanstack/react-router"

import { SourceLink } from "@/datasources/source-menu"
import type { DatasourceStatus } from "@/datasources/queries"
import { exploreText, type ExploreKind } from "@/datasources/sources"
import { cn } from "@/lib/utils"

import { SubNavList, subNavLinkClass, subNavMoreClass } from "./sub-nav"

export function SourceList({
  kind,
  shown,
  more,
  currentId,
  onNavigate,
}: {
  kind: ExploreKind
  shown: readonly DatasourceStatus[]
  more: boolean
  currentId: string | null
  onNavigate?: () => void
}) {
  if (shown.length === 0) return null
  const { word, sources, path } = exploreText[kind]

  return (
    <SubNavList label={sources}>
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
            to={path}
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
