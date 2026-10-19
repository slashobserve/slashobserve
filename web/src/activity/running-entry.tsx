import { useQuery } from "@tanstack/react-query"

import { observeRequestQuery } from "@/observe/queries"

import { DetailsLink, EntryBody, EntryFrame } from "./entry-frame"
import { currentStep, type TimelineEntry } from "./timeline-model"

type Props = {
  entry: Extract<TimelineEntry, { kind: "running" }>
}

export function RunningEntry({ entry }: Props) {
  const { request } = entry
  const { data } = useQuery(observeRequestQuery(request.id))
  const steps = data?.steps ?? []
  const last = steps.at(-1)
  const idle = (data?.status ?? request.status) === "queued"

  return (
    <EntryFrame
      avatar={{ kind: "agent" }}
      who="Agent"
      verb="is working on it"
      at={entry.at}
    >
      <EntryBody className="border-dashed" aria-live="polite">
        {last === undefined
          ? idle
            ? "Waiting to start"
            : "Starting"
          : `${last.message}, step ${currentStep(steps)}`}
      </EntryBody>
      <DetailsLink
        requestId={request.id}
        label="Details of the running agent request, opens in a new tab"
      />
    </EntryFrame>
  )
}
