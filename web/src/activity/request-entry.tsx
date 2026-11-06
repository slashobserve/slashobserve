import { Link } from "@tanstack/react-router"

import { tookText } from "@/chat/turn-model"

import {
  EntryBody,
  EntryFrame,
  EntryMeta,
  MetaState,
  RequestFailure,
  ResolvedState,
  SentToAgent,
} from "./entry-frame"
import type { TimelineEntry } from "./timeline-model"

type Props = {
  dashboardId: string
  entry: Extract<TimelineEntry, { kind: "request" }>
  panelTitle: string | null
  onSelectPanel: (panelId: string) => void
}

function SavedVersionLink({
  dashboardId,
  version,
}: {
  dashboardId: string
  version: number
}) {
  return (
    <Link
      to="/d/$dashboardId/versions"
      params={{ dashboardId }}
      search={
        version > 1 ? { from: version - 1, to: version } : { to: version }
      }
      className="rounded-sm text-foreground underline outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <ResolvedState text={`Saved as version ${version}`} />
    </Link>
  )
}

export function RequestEntry({
  dashboardId,
  entry,
  panelTitle,
  onSelectPanel,
}: Props) {
  const { request, panelId } = entry
  const panel =
    panelId !== null && panelTitle !== null
      ? { title: panelTitle, onSelect: () => onSelectPanel(panelId) }
      : undefined
  const verb =
    panelId === null
      ? "asked the agent"
      : panel === undefined
        ? "asked the agent on a removed panel"
        : "asked the agent on"

  let state
  switch (request.status) {
    case "queued":
    case "running":
      state = <SentToAgent />
      break
    case "succeeded":
      state =
        request.result_version === null ? (
          <MetaState>Finished, nothing changed</MetaState>
        ) : (
          <SavedVersionLink
            dashboardId={dashboardId}
            version={request.result_version}
          />
        )
      break
    case "failed":
      state = <RequestFailure error={request.error} />
      break
  }

  const took = tookText(request.started_at, request.finished_at)

  return (
    <EntryFrame
      avatar={{ kind: "person", name: request.requested_by.name }}
      who={request.requested_by.name}
      verb={verb}
      panel={panel}
      at={request.created_at}
    >
      <EntryBody>{request.prompt}</EntryBody>
      <EntryMeta>
        {state}
        {took !== undefined ? <MetaState>Took {took}</MetaState> : null}
      </EntryMeta>
    </EntryFrame>
  )
}
