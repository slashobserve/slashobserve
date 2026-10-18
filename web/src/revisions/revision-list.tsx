import { useMemo } from "react"
import { BotIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import { formatDateTime, formatRelative } from "@/lib/time"

import {
  isActive,
  useAgentAvailable,
  useRevisions,
  type Revision,
} from "./queries"
import { RevisionStatus } from "./revision-status"

const shown = 10

type PanelRef = { id: string; title: string }

function sourceText(revision: Revision, panels: PanelRef[]): string {
  if (revision.source !== "comment") return "Prompt"
  const title =
    panels.find((panel) => panel.id === revision.panel_id)?.title ??
    revision.panel_id
  return title === null ? "Comment" : `Comment on ${title}`
}

export function useHasRevisionList(dashboardId: string): boolean {
  const available = useAgentAvailable()
  const { data } = useRevisions(dashboardId)
  return available || (data?.length ?? 0) > 0
}

export function RevisionList({
  dashboardId,
  panels,
}: {
  dashboardId: string
  panels: PanelRef[]
}) {
  const visible = useHasRevisionList(dashboardId)
  const { data } = useRevisions(dashboardId)
  const revisions = useMemo(() => (data ?? []).slice(0, shown), [data])
  const waiting = useMemo(
    () => (data ?? []).filter((revision) => isActive(revision.status)).length,
    [data]
  )

  if (!visible) return null

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            aria-label={
              waiting > 0 ? `Revisions, ${waiting} in progress` : "Revisions"
            }
          />
        }
      >
        <BotIcon data-icon="inline-start" />
        Revisions
        {waiting > 0 ? (
          <Badge variant="secondary" className="tabular-nums">
            {waiting}
          </Badge>
        ) : null}
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="max-h-[min(32rem,calc(100dvh-6rem))] w-[min(28rem,calc(100vw-2rem))] overflow-y-auto"
      >
        <PopoverTitle>Revisions</PopoverTitle>
        {revisions.length === 0 ? (
          <p className="text-muted-foreground">No revisions yet.</p>
        ) : (
          <ul className="flex flex-col divide-y">
            {revisions.map((revision) => (
              <li
                key={revision.id}
                className="flex flex-col gap-1 py-2 first:pt-0 last:pb-0"
              >
                <p className="text-xs wrap-anywhere text-muted-foreground">
                  {sourceText(revision, panels)}
                </p>
                <p
                  title={revision.prompt}
                  className="line-clamp-2 text-sm wrap-anywhere"
                >
                  {revision.prompt}
                </p>
                <p className="text-xs text-muted-foreground">
                  <span className="wrap-anywhere">
                    {revision.requested_by.name}
                  </span>
                  {" - "}
                  <time
                    dateTime={revision.created_at}
                    title={formatDateTime(revision.created_at)}
                  >
                    {formatRelative(revision.created_at)}
                  </time>
                </p>
                <RevisionStatus revision={revision} dashboardId={dashboardId} />
              </li>
            ))}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  )
}
