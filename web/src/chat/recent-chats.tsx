import { Link, useNavigate } from "@tanstack/react-router"
import { useId } from "react"

import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { requestStatusView, useMyRequests } from "@/observe/queries"

const recentLimit = 5

export function RecentChats() {
  const navigate = useNavigate()
  const headingId = useId()
  const { requests, query } = useMyRequests("observe", recentLimit)

  const failed = query.data === undefined && !query.isPending

  if (query.data !== undefined && requests.length === 0) return null

  return (
    <section aria-labelledby={headingId} className="mt-7 w-full max-w-[720px]">
      <div className="flex items-center justify-between px-0.5 pb-2">
        <h2
          id={headingId}
          className="text-[13px] font-medium text-muted-foreground"
        >
          Recent chats
        </h2>
        <Link
          to="/dashboards"
          className="rounded-control text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        >
          All dashboards
        </Link>
      </div>
      {failed ? (
        <div className="flex items-center gap-3 rounded-card border bg-background px-3.5 py-2.5">
          <p className="text-xs text-muted-foreground">
            Could not load recent chats.
          </p>
          <Button
            variant="outline"
            size="xs"
            disabled={query.isFetching}
            focusableWhenDisabled
            onClick={() => void query.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : (
        <ul
          aria-busy={query.isPending}
          className="overflow-hidden rounded-card border bg-background"
        >
          {query.isPending
            ? Array.from({ length: 4 }, (_, index) => (
                <li key={index} className="border-b last:border-b-0">
                  <Skeleton className="h-[39px] rounded-none" />
                </li>
              ))
            : requests.map((request) => {
                const view = requestStatusView(request)
                return (
                  <li key={request.id} className="border-b last:border-b-0">
                    <button
                      type="button"
                      className="flex w-full flex-wrap items-center gap-x-3 gap-y-0.5 px-3.5 py-2.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
                      onClick={() =>
                        void navigate({
                          to: "/observe/$requestId",
                          params: { requestId: request.id },
                        })
                      }
                    >
                      <span className="min-w-0 flex-1 truncate text-[13px] narrow:basis-full">
                        {request.prompt}
                      </span>
                      <span className="flex flex-none items-center text-xs text-muted-foreground">
                        <StatusLabel tone={view.tone}>
                          <span className="max-w-[220px] truncate narrow:max-w-[180px]">
                            {view.text}
                          </span>
                        </StatusLabel>
                      </span>
                      <span className="w-20 flex-none text-right text-xs text-muted-foreground narrow:ml-auto narrow:w-auto">
                        <RelativeTime iso={request.created_at} />
                      </span>
                    </button>
                  </li>
                )
              })}
        </ul>
      )}
    </section>
  )
}
