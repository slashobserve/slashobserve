import { Link, useNavigate } from "@tanstack/react-router"
import { LayoutDashboardIcon, NotebookTextIcon } from "lucide-react"

import { chatOutcome } from "@/chat/chat-state"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { InboxRow } from "@/inbox/inbox-row"
import { useInboxForYou } from "@/inbox/queries"
import { dashboardLink, observationLink } from "@/lib/links"
import { useMyRequests } from "@/observe/queries"

import { activeRuns, needsYou, pickUp } from "./home-lists"
import {
  HomeSection,
  IconTile,
  RowText,
  SectionLink,
  SectionList,
  rowLinkClass,
} from "./home-section"
import { useRecent } from "./recent"

export function NeedsYouList() {
  const navigate = useNavigate()
  const query = useInboxForYou()
  const items =
    query.data === undefined
      ? undefined
      : needsYou(query.data.pages[0]?.items ?? [], 3)

  return (
    <HomeSection
      title="Needs you"
      action={<SectionLink to="/inbox">Open Inbox</SectionLink>}
    >
      <SectionList
        items={items}
        query={query}
        getKey={(item) => String(item.seq)}
        emptyText="Nothing needs you right now."
        errorText="Could not load the inbox."
        renderRow={(item) => (
          <div className="p-1">
            <InboxRow
              item={item}
              selected={false}
              tabStop
              asLink
              onSelect={(selected) =>
                void navigate({
                  to: "/inbox",
                  search: { item: String(selected.seq) },
                })
              }
            />
          </div>
        )}
      />
    </HomeSection>
  )
}

export function WorkingNowList() {
  const { requests, query } = useMyRequests({}, 20)
  const items = query.data === undefined ? undefined : activeRuns(requests)

  return (
    <HomeSection
      title="Working now"
      action={<SectionLink to="/agents">All agents</SectionLink>}
    >
      <SectionList
        items={items}
        query={query}
        getKey={(request) => request.id}
        emptyText="Nothing is running."
        errorText="Could not load your runs."
        renderRow={(request) => (
          <Link
            to="/c/$requestId"
            params={{ requestId: request.id }}
            className={rowLinkClass}
          >
            <RowText
              title={request.title}
              meta={
                <>
                  <StatusLabel tone="running">Running</StatusLabel>
                  <span>{chatOutcome(request)}</span>
                  {request.dashboard_title === null ? null : (
                    <span>on {request.dashboard_title}</span>
                  )}
                  {request.origin === "mcp" &&
                  request.origin_client !== null ? (
                    <span>via {request.origin_client}</span>
                  ) : null}
                </>
              }
            />
            <span className="flex-none text-xs text-muted-foreground">
              <RelativeTime iso={request.started_at ?? request.created_at} />
            </span>
          </Link>
        )}
      />
    </HomeSection>
  )
}

export function PickUpList() {
  const items = pickUp(useRecent("dashboard"), useRecent("observation"), 5)

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
                <span>
                  {entry.kind === "dashboard" ? "Dashboard" : "Observation"}
                  {" · Viewed "}
                  <RelativeTime iso={entry.viewed_at} />
                </span>
              }
            />
          </Link>
        )}
      />
    </HomeSection>
  )
}
