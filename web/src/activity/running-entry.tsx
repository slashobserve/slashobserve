import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"

import { observeRequestQuery } from "@/observe/queries"

import { boxClass, EntryFrame } from "./entry-frame"
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
      <Link
        to="/observe/$requestId"
        params={{ requestId: request.id }}
        target="_blank"
        rel="noopener"
        aria-label="Details of the running agent request, opens in a new tab"
        className={`${boxClass} flex items-center gap-2 border-dashed no-underline outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring`}
      >
        <svg
          width={13}
          height={13}
          viewBox="0 0 24 24"
          fill="none"
          strokeWidth={2.4}
          strokeLinecap="round"
          aria-hidden="true"
          className="flex-none stroke-brand motion-safe:animate-spin"
        >
          <path d="M21 12a9 9 0 1 1-6.2-8.6" />
        </svg>
        <span className="min-w-0 flex-1 wrap-anywhere" aria-live="polite">
          {last === undefined
            ? idle
              ? "Waiting to start"
              : "Starting"
            : `${last.message}, step ${currentStep(steps)}`}
        </span>
        <span className="flex-none text-xs text-muted-foreground">Details</span>
      </Link>
    </EntryFrame>
  )
}
