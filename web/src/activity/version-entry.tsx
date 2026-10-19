import { Link } from "@tanstack/react-router"

import { versionActor } from "@/lib/origin"

import { boxClass, DetailsLink, EntryFrame } from "./entry-frame"
import type { TimelineEntry } from "./timeline-model"

type Props = {
  dashboardId: string
  entry: Extract<TimelineEntry, { kind: "version" }>
  back: string | undefined
}

export function VersionEntry({ dashboardId, entry, back }: Props) {
  const { version } = entry
  const n = version.version
  const actor = versionActor(version)

  return (
    <EntryFrame
      avatar={
        actor.agent
          ? { kind: "agent" }
          : { kind: "person", name: version.author.name }
      }
      who={actor.who}
      verb={actor.verb}
      at={entry.at}
    >
      <Link
        to="/dashboards/$dashboardId/versions"
        params={{ dashboardId }}
        search={n > 1 ? { from: n - 1, to: n, back } : { to: n, back }}
        className={`${boxClass} flex items-center justify-between gap-2.5 no-underline outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring`}
      >
        <span className="min-w-0">
          <span className="font-medium">Version {n}</span>
          <span className="block text-xs wrap-anywhere text-muted-foreground">
            {version.summary.trim() === "" ? "No summary" : version.summary}
          </span>
        </span>
        <span className="flex-none text-xs text-muted-foreground">
          See changes
        </span>
      </Link>
      {version.agent_request_id === null ? null : (
        <DetailsLink
          requestId={version.agent_request_id}
          label={`Agent run details for version ${n}, opens in a new tab`}
        />
      )}
    </EntryFrame>
  )
}
