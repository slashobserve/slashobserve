import { useId, useRef } from "react"
import { useNavigate } from "@tanstack/react-router"
import { HistoryIcon } from "lucide-react"

import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  requestStatusView,
  useMyRequests,
  type RequestSummary,
} from "@/observe/queries"
import { SidePanel } from "@/shell/side-panel"
import { TopRowActions } from "@/shell/top-row-actions"

import { useHistoryPanel } from "./history-store"

const pageSize = 20

export function ChatHistory({
  currentRequestId,
}: {
  currentRequestId?: string
}) {
  const open = useHistoryPanel((state) => state.open)
  const setOpen = useHistoryPanel((state) => state.setOpen)
  const panelId = useId()
  const trigger = useRef<HTMLButtonElement>(null)

  return (
    <>
      <TopRowActions>
        <Button
          ref={trigger}
          variant="ghost"
          size="sm"
          className="h-[30px] gap-1.5 text-[13px]"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => setOpen(!open)}
        >
          <HistoryIcon aria-hidden="true" />
          History
        </Button>
      </TopRowActions>
      {open ? (
        <SidePanel
          title="History"
          description="Your requests, newest first."
          trigger={trigger}
          onClose={() => setOpen(false)}
        >
          <HistoryList panelId={panelId} currentRequestId={currentRequestId} />
        </SidePanel>
      ) : null}
    </>
  )
}

function HistoryList({
  panelId,
  currentRequestId,
}: {
  panelId: string
  currentRequestId?: string
}) {
  const navigate = useNavigate()
  const { requests, query } = useMyRequests("all", pageSize)

  if (query.isPending) {
    return (
      <div id={panelId} aria-busy="true" className="flex flex-col gap-2">
        {Array.from({ length: 5 }, (_, index) => (
          <Skeleton key={index} className="h-14" />
        ))}
      </div>
    )
  }

  if (query.data === undefined) {
    return (
      <div id={panelId} className="flex flex-col items-start gap-3">
        <p className="text-[13px] text-muted-foreground">
          Could not load your requests.
        </p>
        <Button
          variant="outline"
          size="sm"
          disabled={query.isFetching}
          focusableWhenDisabled
          onClick={() => void query.refetch()}
        >
          Retry
        </Button>
      </div>
    )
  }

  if (requests.length === 0) {
    return (
      <p id={panelId} className="text-[13px] text-muted-foreground">
        No requests yet. Ask for a dashboard to start one.
      </p>
    )
  }

  return (
    <div id={panelId} className="flex flex-col gap-3">
      <ul className="-mx-2 flex flex-col">
        {requests.map((request) => (
          <li key={request.id}>
            <button
              type="button"
              aria-current={
                request.id === currentRequestId ? "true" : undefined
              }
              className="flex w-full flex-col gap-1 rounded-control px-2 py-2.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-[current=true]:bg-muted"
              onClick={() =>
                void navigate({
                  to: "/observe/$requestId",
                  params: { requestId: request.id },
                })
              }
            >
              <span className="line-clamp-2 text-[13px] wrap-anywhere">
                {request.prompt}
              </span>
              <HistoryMeta request={request} />
            </button>
          </li>
        ))}
      </ul>
      {query.hasNextPage ? (
        <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-2">
          <Button
            variant="outline"
            size="sm"
            aria-busy={query.isFetchingNextPage}
            disabled={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Load more
          </Button>
          {query.isFetchNextPageError ? (
            <p role="alert" className="text-xs text-destructive">
              Could not load more requests.
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function HistoryMeta({ request }: { request: RequestSummary }) {
  const view = requestStatusView(request)
  const statusText =
    request.status === "succeeded"
      ? request.result_version !== null
        ? `Saved version ${request.result_version}`
        : "Finished"
      : view.text

  return (
    <span className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
      <StatusLabel tone={view.tone}>
        {request.status === "failed" ? (
          <span className="max-w-[260px] truncate">{statusText}</span>
        ) : (
          statusText
        )}
      </StatusLabel>
      {request.dashboard_title !== null ? (
        <span className="max-w-[200px] truncate">
          {request.dashboard_title}
        </span>
      ) : null}
      <RelativeTime iso={request.created_at} />
    </span>
  )
}
