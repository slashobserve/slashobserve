import { Link } from "@tanstack/react-router"
import {
  CircleAlertIcon,
  CircleCheckIcon,
  ClockIcon,
  LoaderCircleIcon,
} from "lucide-react"

import { searchFromState, useDashboardSearch } from "@/dashboards/search"

import { useLatestVersion, type RevisionProgress } from "./queries"

type Props = {
  revision: RevisionProgress
  dashboardId: string
  live?: boolean
}

const iconClass = "size-4 shrink-0"

export function RevisionStatus({ revision, dashboardId, live = false }: Props) {
  const search = useDashboardSearch()
  const latestVersion = useLatestVersion(dashboardId)
  const { status } = revision
  const created = revision.result_version ?? null

  const stepsLink =
    status === "succeeded" ? null : (
      <Link
        to="/observe/$requestId"
        params={{ requestId: revision.id }}
        className="text-xs text-muted-foreground underline underline-offset-3 hover:text-foreground"
      >
        View progress
      </Link>
    )

  let body
  if (status === "queued") {
    body = (
      <>
        <ClockIcon aria-hidden className={iconClass} />
        <span>Queued for the agent</span>
      </>
    )
  } else if (status === "running") {
    body = (
      <>
        <LoaderCircleIcon
          aria-hidden
          className={`${iconClass} motion-safe:animate-spin`}
        />
        <span>
          Agent is working
          {(revision.attempts ?? 0) > 1
            ? ` (attempt ${revision.attempts})`
            : ""}
        </span>
      </>
    )
  } else if (status === "succeeded") {
    body = (
      <>
        <CircleCheckIcon
          aria-hidden
          className={`${iconClass} text-threshold-ok`}
        />
        {created === null ? (
          <span>Revision applied</span>
        ) : (
          <Link
            to="/dashboards/$dashboardId"
            params={{ dashboardId }}
            search={searchFromState({
              ...search,
              version: created === latestVersion ? undefined : created,
              panel: undefined,
              tab: undefined,
            })}
            className="underline underline-offset-3 hover:text-foreground"
          >
            Created version {created}
          </Link>
        )}
      </>
    )
  } else {
    body = (
      <>
        <CircleAlertIcon
          aria-hidden
          className={`${iconClass} text-destructive`}
        />
        <span>Revision failed</span>
      </>
    )
  }

  return (
    <div
      role={live ? "status" : undefined}
      data-revision-status={status}
      className="flex min-w-0 flex-col gap-0.5 text-sm"
    >
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        {body}
        {stepsLink}
      </div>
      {status === "failed" && revision.error ? (
        <p className="pl-6 text-xs wrap-anywhere text-muted-foreground">
          {revision.error}
        </p>
      ) : null}
    </div>
  )
}
