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
  entry: Extract<TimelineEntry, { kind: "request" }>
  panelTitle: string | null
  onSelectPanel: (panelId: string) => void
}

export function RequestEntry({ entry, panelTitle, onSelectPanel }: Props) {
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
          <ResolvedState
            text={`Resolved by version ${request.result_version}`}
          />
        )
      break
    case "failed":
      state = <RequestFailure error={request.error} />
      break
  }

  return (
    <EntryFrame
      avatar={{ kind: "person", name: request.requested_by.name }}
      who={request.requested_by.name}
      verb={verb}
      panel={panel}
      at={request.created_at}
    >
      <EntryBody>{request.prompt}</EntryBody>
      <EntryMeta>{state}</EntryMeta>
    </EntryFrame>
  )
}
