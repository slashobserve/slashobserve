import { useInfiniteQuery } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { NotebookTextIcon, UploadIcon } from "lucide-react"

import { chatOutcome, chatState } from "@/chat/chat-state"
import { useHistoryPanel } from "@/chat/history-store"
import { AuthorMark } from "@/components/author-mark"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { myImportsQuery } from "@/import/record-queries"
import { importStatus } from "@/import/record-summary"
import { markOf } from "@/lib/author"
import { dashboardLink, observationLink } from "@/lib/links"
import { observationsInfiniteQuery } from "@/observations/queries"
import { useMyRequests } from "@/observe/queries"

import { createdBy, newestChats } from "./home-lists"
import {
  HomeSection,
  IconTile,
  RowText,
  SectionLink,
  SectionList,
  rowLinkClass,
  textLinkClass,
} from "./home-section"

const shown = 5
const observationsFetched = 20

function Time({ iso }: { iso: string }) {
  return (
    <span className="flex-none text-xs text-muted-foreground">
      <RelativeTime iso={iso} />
    </span>
  )
}

export function RecentChatsList() {
  const { requests, query } = useMyRequests({}, 20)
  const setTab = useHistoryPanel((state) => state.setTab)
  const setOpen = useHistoryPanel((state) => state.setOpen)
  const items =
    query.data === undefined ? undefined : newestChats(requests, shown)

  return (
    <HomeSection
      title="Recent chats"
      action={
        <button
          type="button"
          className={textLinkClass}
          onClick={() => {
            setTab("chats")
            setOpen(true)
          }}
        >
          All chats
        </button>
      }
    >
      <SectionList
        items={items}
        query={query}
        getKey={(request) => request.id}
        emptyText="No requests yet. Ask for a dashboard to start one."
        errorText="Could not load your chats."
        renderRow={(request) => {
          const state = chatState(request.state)
          return (
            <Link
              to="/c/$requestId"
              params={{ requestId: request.id }}
              className={rowLinkClass}
            >
              <RowText
                title={request.title}
                meta={
                  <>
                    <StatusLabel tone={state.tone}>{state.word}</StatusLabel>
                    <span>{chatOutcome(request)}</span>
                  </>
                }
              />
              <Time iso={request.created_at} />
            </Link>
          )
        }}
      />
    </HomeSection>
  )
}

export function OpenObservationsList() {
  const { principal } = useRouteContext({ from: "/_authed" })
  const query = useInfiniteQuery(
    observationsInfiniteQuery({ state: "open" }, observationsFetched)
  )
  const first = query.data?.pages[0]
  const items =
    first === undefined
      ? undefined
      : createdBy(first.observations, principal.id, shown)

  return (
    <HomeSection
      title="Your open observations"
      action={<SectionLink to="/observations">All observations</SectionLink>}
    >
      <SectionList
        items={items}
        query={query}
        getKey={(observation) => observation.id}
        emptyText="You have no open observations."
        errorText="Could not load open observations."
        renderRow={(observation) => (
          <Link {...observationLink(observation.id)} className={rowLinkClass}>
            <IconTile>
              <NotebookTextIcon />
            </IconTile>
            <RowText
              title={observation.title}
              meta={
                <>
                  <span>Started by {observation.created_by.label}</span>
                  <RelativeTime iso={observation.created_at} />
                </>
              }
            />
            <AuthorMark mark={markOf(observation.created_by)} size={20} />
          </Link>
        )}
      />
    </HomeSection>
  )
}

export function RecentImportsList() {
  const query = useInfiniteQuery(myImportsQuery())
  const first = query.data?.pages[0]
  const items = first?.imports.slice(0, shown)

  return (
    <HomeSection
      title="Recent imports"
      action={<SectionLink to="/dashboards">All dashboards</SectionLink>}
    >
      <SectionList
        items={items}
        query={query}
        getKey={(entry) => entry.id}
        emptyText="No imports yet."
        errorText="Could not load your imports."
        renderRow={(entry) => {
          const status = importStatus(entry.counts)
          return (
            <Link
              {...dashboardLink(entry.dashboard_id, entry.title)}
              className={rowLinkClass}
            >
              <IconTile>
                <UploadIcon />
              </IconTile>
              <RowText
                title={entry.title}
                meta={
                  <>
                    <StatusLabel tone={status.tone}>{status.text}</StatusLabel>
                    <span>
                      {entry.counts.total}{" "}
                      {entry.counts.total === 1 ? "panel" : "panels"}
                    </span>
                  </>
                }
              />
              <Time iso={entry.created_at} />
            </Link>
          )
        }}
      />
    </HomeSection>
  )
}
