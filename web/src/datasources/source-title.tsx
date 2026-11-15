import { TopRowTitle } from "@/shell/top-row-title"

import type { DatasourceStatus } from "./queries"
import { sourceLanguage, type ExploreKind } from "./sources"

export function SourceTitle({
  kind,
  source,
}: {
  kind: ExploreKind
  source: DatasourceStatus
}) {
  return (
    <TopRowTitle>
      <span className="flex min-w-0 items-center gap-1.5">
        <span aria-current="page" className="truncate font-medium">
          {source.name}
        </span>
        <span className="rounded-chip bg-muted px-1.5 text-xs text-muted-foreground">
          {sourceLanguage(kind, source.type)}
        </span>
      </span>
    </TopRowTitle>
  )
}
