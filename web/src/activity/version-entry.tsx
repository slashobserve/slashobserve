import { Link } from "@tanstack/react-router"
import { ArrowRightIcon } from "lucide-react"

import { versionActor } from "@/lib/origin"

import { DetailsLink, EntryFrame } from "./entry-frame"
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
      avatar={{ kind: "version" }}
      who={actor.who}
      verb={actor.verb}
      at={entry.at}
    >
      <Link
        to="/dashboards/$dashboardId/versions"
        params={{ dashboardId }}
        search={n > 1 ? { from: n - 1, to: n, back } : { to: n, back }}
        className="mt-1.5 flex items-center gap-3 rounded-control border px-2.5 py-2 text-[13px] no-underline outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span className="flex min-w-0 flex-col">
          <span className="font-semibold">Version {n}</span>
          <span className="wrap-anywhere text-muted-foreground">
            {version.summary.trim() === "" ? "No summary" : version.summary}
          </span>
        </span>
        <span className="ml-auto inline-flex flex-none items-center gap-1.5 text-muted-foreground">
          See changes
          <ArrowRightIcon className="size-3.5" />
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
