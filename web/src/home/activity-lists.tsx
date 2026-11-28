import type { ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate, useRouteContext } from "@tanstack/react-router"
import { LayoutDashboardIcon, NotebookTextIcon } from "lucide-react"
import { toast } from "sonner"

import { AuthorMark } from "@/components/author-mark"
import { RelativeTime, useNow } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button, buttonVariants } from "@/components/ui/button"
import { dashboardsHealthQuery } from "@/health/queries"
import { InboxRow } from "@/inbox/inbox-row"
import { itemText, type InboxItem } from "@/inbox/inbox-item"
import { useInboxPanelStore } from "@/inbox/inbox-panel-store"
import { useInboxForYou, useSetInboxItemUnread } from "@/inbox/queries"
import { dashboardLink, observationLink } from "@/lib/links"
import { describeSubmitError, useRetryRequest } from "@/observe/queries"
import { pinProblemText } from "@/pins/pin-problems"
import { cn } from "@/lib/utils"

import { changeText } from "./change-text"
import {
  agentMark,
  needsYou,
  pickUp,
  runningText,
  workingPlaces,
  workingTarget,
  workingTitle,
  type PickUpEntry,
  type WorkingRun,
  type WorkingTarget,
} from "./home-lists"
import {
  HomeSection,
  IconTile,
  RowText,
  SectionButton,
  SectionLink,
  SectionList,
  rowLinkClass,
} from "./home-section"
import { needsYouAction, type NeedsYouAction } from "./needs-you-action"
import { latestChangeQuery, workingRunsQuery } from "./queries"
import { useRecent } from "./recent"

function RetryAction({
  requestId,
  label,
}: {
  requestId: string
  label: string
}) {
  const navigate = useNavigate()
  const retry = useRetryRequest(requestId)
  return (
    <Button
      variant="outline"
      size="xs"
      disabled={retry.isPending}
      aria-label={label}
      onClick={() =>
        retry.mutate(undefined, {
          onSuccess: (request) =>
            void navigate({
              to: "/c/$requestId",
              params: { requestId: request.id },
            }),
          onError: (error) => toast.error(describeSubmitError(error).message),
        })
      }
    >
      Try again
    </Button>
  )
}

function ActionControl({
  action,
  item,
}: {
  action: NeedsYouAction
  item: InboxItem
}) {
  const label = (word: string) => `${word}: ${itemText(item).object}`
  if (action.kind === "retry") {
    return (
      <RetryAction requestId={action.requestId} label={label("Try again")} />
    )
  }
  return (
    <Link
      to={action.to.to}
      {...(action.to.search === undefined ? {} : { search: action.to.search })}
      aria-label={label(action.label)}
      className={cn(buttonVariants({ variant: "outline", size: "xs" }))}
    >
      {action.label}
    </Link>
  )
}

function NeedsYouRow({ item }: { item: InboxItem }) {
  const showInbox = useInboxPanelStore((state) => state.show)
  const setUnread = useSetInboxItemUnread()
  const action = needsYouAction(item)
  function markRead() {
    if (item.unread) setUnread.mutate({ seq: item.seq, unread: false })
  }
  return (
    <div className="flex items-center gap-2 p-1 pr-3">
      <div className="min-w-0 flex-1">
        <InboxRow
          item={item}
          selected={false}
          tabStop
          asLink
          onOpen={markRead}
          onSelect={() => {
            markRead()
            showInbox({ view: "for_you", filter: "open", item: item.seq })
          }}
        />
      </div>
      {action === null ? null : (
        <div className="flex-none">
          <ActionControl action={action} item={item} />
        </div>
      )}
    </div>
  )
}

export function NeedsYouList() {
  const showInbox = useInboxPanelStore((state) => state.show)
  const query = useInboxForYou()
  const items =
    query.data === undefined
      ? undefined
      : needsYou(query.data.pages[0]?.items ?? [], 3)

  return (
    <HomeSection
      title="Needs you"
      action={
        <SectionButton onClick={() => showInbox()}>Open Inbox</SectionButton>
      }
    >
      <SectionList
        items={items}
        query={query}
        getKey={(item) => String(item.seq)}
        emptyText="Nothing needs you right now."
        errorText="Could not load the inbox."
        renderRow={(item) => <NeedsYouRow item={item} />}
      />
    </HomeSection>
  )
}

function TargetLink({
  target,
  children,
}: {
  target: WorkingTarget | null
  children: ReactNode
}) {
  if (target === null) {
    return (
      <div className="flex w-full items-center gap-3 px-4 py-2.5 narrow:min-h-11">
        {children}
      </div>
    )
  }
  switch (target.kind) {
    case "chat":
      return (
        <Link
          to="/c/$requestId"
          params={{ requestId: target.id }}
          className={rowLinkClass}
        >
          {children}
        </Link>
      )
    case "dashboard":
      return (
        <Link
          {...dashboardLink(target.id, target.title)}
          className={rowLinkClass}
        >
          {children}
        </Link>
      )
    case "observation":
      return (
        <Link {...observationLink(target.id)} className={rowLinkClass}>
          {children}
        </Link>
      )
  }
}

function WorkingRow({ run }: { run: WorkingRun }) {
  const now = useNow()
  return (
    <TargetLink target={workingTarget(run)}>
      <AuthorMark mark={agentMark(run.agent)} size={20} />
      <RowText
        title={`${run.agent.name} is working on ${workingTitle(run)}`}
        meta={
          <>
            <StatusLabel tone="running">{runningText(run, now)}</StatusLabel>
            <span>{run.requested_by.name}</span>
            {workingPlaces(run).map((place) => (
              <span key={place}>{place}</span>
            ))}
          </>
        }
      />
    </TargetLink>
  )
}

export function WorkingNowList() {
  const query = useQuery(workingRunsQuery())
  const items = query.data?.items

  return (
    <HomeSection
      title="Working now"
      action={<SectionLink to="/agents">All agents</SectionLink>}
    >
      <SectionList
        items={items}
        query={query}
        getKey={(run) => run.id}
        emptyText="Nothing is running."
        errorText="Could not load the running work."
        renderRow={(run) => <WorkingRow run={run} />}
      />
    </HomeSection>
  )
}

function PickUpMeta({
  entry,
  problem,
}: {
  entry: PickUpEntry
  problem: string | null
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const now = useNow()
  const change = useQuery({
    ...latestChangeQuery(entry.id),
    enabled: entry.kind === "observation",
  }).data
  const changed =
    change === undefined || change === null
      ? null
      : changeText(change, principal.id, now)
  const extra = entry.kind === "dashboard" ? problem : changed

  return (
    <span>
      {entry.kind === "dashboard" ? "Dashboard" : "Observation"}
      {" · Viewed "}
      <RelativeTime iso={entry.viewed_at} />
      {extra === null ? null : ` · ${extra}`}
    </span>
  )
}

export function PickUpList() {
  const items = pickUp(useRecent("dashboard"), useRecent("observation"), 5)
  const health = useQuery(
    dashboardsHealthQuery(
      items.filter((entry) => entry.kind === "dashboard").map(({ id }) => id)
    )
  ).data

  return (
    <HomeSection title="Pick up where you left off">
      <SectionList
        items={items}
        getKey={(entry) => `${entry.kind}:${entry.id}`}
        emptyText="Dashboards and observations you open show here."
        renderRow={(entry) => (
          <Link
            {...(entry.kind === "dashboard"
              ? dashboardLink(entry.id, entry.title)
              : observationLink(entry.id))}
            className={rowLinkClass}
          >
            <IconTile>
              {entry.kind === "dashboard" ? (
                <LayoutDashboardIcon />
              ) : (
                <NotebookTextIcon />
              )}
            </IconTile>
            <RowText
              title={entry.title}
              meta={
                <PickUpMeta
                  entry={entry}
                  problem={pinProblemText(health?.get(entry.id))}
                />
              }
            />
          </Link>
        )}
      />
    </HomeSection>
  )
}
