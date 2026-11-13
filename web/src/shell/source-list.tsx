import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { SourceFlyoutItems, SourceLink } from "@/datasources/source-menu"
import type { DatasourceStatus } from "@/datasources/queries"
import type { ExploreKind } from "@/datasources/sources"
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
  const word = kind === "logs" ? "log" : "trace"

  return (
    <SubNavList label={kind === "logs" ? "Log sources" : "Trace sources"}>
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
          <DropdownMenu modal={false}>
            <DropdownMenuTrigger
              render={
                <button
                  type="button"
                  aria-label={`More ${word} sources`}
                  className={cn(subNavMoreClass, "w-full")}
                />
              }
            >
              More
            </DropdownMenuTrigger>
            <DropdownMenuContent
              side="right"
              align="start"
              className="w-[248px] rounded-card p-1"
            >
              <SourceFlyoutItems kind={kind} />
            </DropdownMenuContent>
          </DropdownMenu>
        </li>
      ) : null}
    </SubNavList>
  )
}
