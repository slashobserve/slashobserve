import { TopRowTitle } from "@/shell/top-row-title"

import type { DatasourceStatus } from "./queries"
import { usePublishSourceLanguage } from "./source-language-store"
import { sourceLanguage, type ExploreKind } from "./sources"

export function SourceTitle({
  kind,
  source,
  language = sourceLanguage(kind, source.type),
}: {
  kind: ExploreKind
  source: DatasourceStatus
  language?: string | undefined
}) {
  usePublishSourceLanguage(source.uid, language)
  return (
    <TopRowTitle>
      <span className="flex min-w-0 items-center gap-1.5">
        <span aria-current="page" className="truncate font-medium">
          {source.name}
        </span>
        <span className="rounded-chip bg-muted px-1.5 text-xs text-muted-foreground">
          {language}
        </span>
      </span>
    </TopRowTitle>
  )
}
