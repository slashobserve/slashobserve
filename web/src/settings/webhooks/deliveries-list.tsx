import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Skeleton } from "@/components/ui/skeleton"
import { SettingsProblem } from "@/settings/settings-card"

import { eventTypeLabels, type ActivityType } from "./event-types"
import { useDeliveries } from "./queries"
import { errorText, outcomeTone } from "./status"
import { webhookProblemText } from "./webhook-problems"

export function DeliveriesList({ webhookId }: { webhookId: string }) {
  const query = useDeliveries(webhookId)
  if (query.isError && query.data === undefined) {
    return (
      <SettingsProblem
        title="Could not load the delivery attempts"
        detail={webhookProblemText(query.error)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }
  if (query.data === undefined) {
    return <Skeleton className="m-3.5 h-4 w-64" />
  }
  if (query.data.length === 0) {
    return (
      <p className="px-3.5 py-3 text-xs text-muted-foreground">
        No attempts in the last 7 days.
      </p>
    )
  }
  return (
    <ul aria-label="Recent delivery attempts" className="divide-y text-xs">
      {query.data.map((attempt) => (
        <li
          key={attempt.id}
          className="flex flex-wrap items-center gap-x-4 gap-y-1 px-3.5 py-2"
        >
          <span className="w-24 text-muted-foreground">
            <RelativeTime iso={attempt.attempted_at} />
          </span>
          <span className="min-w-32">
            {eventTypeLabels[attempt.event_type as ActivityType] ??
              attempt.event_type}
          </span>
          <StatusLabel tone={outcomeTone(attempt.outcome)}>
            {attempt.outcome}
          </StatusLabel>
          <span className="text-muted-foreground">
            {attempt.status_code === null ? "no answer" : attempt.status_code}
            {", "}
            {attempt.duration_ms} ms
          </span>
          {attempt.error_code === null ? null : (
            <span className="text-muted-foreground">
              {errorText(attempt.error_code)}
            </span>
          )}
        </li>
      ))}
    </ul>
  )
}
