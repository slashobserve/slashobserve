import { useInfiniteQuery } from "@tanstack/react-query"
import { Link, useRouteContext } from "@tanstack/react-router"
import { NotebookTextIcon, UploadIcon } from "lucide-react"

import { ChatRowMeta } from "@/chat/chat-row-meta"
import { useChats, useDashboardTitles } from "@/chat/chats-queries"
import { AuthorMark } from "@/components/author-mark"
import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { myImportsQuery } from "@/import/record-queries"
import { markOf, markOfAuthor } from "@/lib/author"
import { dashboardLink, observationLink } from "@/lib/links"
import { observationsInfiniteQuery } from "@/observations/queries"

import { createdBy, importMeta, messageCountText } from "./home-lists"
import {
  HomeSection,
  IconTile,
  RowText,
  SectionLink,
  SectionList,
  rowLinkClass,
} from "./home-section"
import { plainTitle } from "@/comments/plain-title"

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
  const { chats, query } = useChats({}, shown)
  const titles = useDashboardTitles()
  const items = query.data === undefined ? undefined : chats

  return (
    <HomeSection
      title="Recent chats"
      action={<SectionLink to="/chats">All chats</SectionLink>}
    >
      <SectionList
        items={items}
        query={query}
        getKey={(chat) => chat.id}
        emptyText="No chats yet. Ask for a dashboard to start one."
        errorText="Could not load your chats."
        renderRow={(chat) => (
          <Link
            to="/c/$requestId"
            params={{ requestId: chat.id }}
            className={rowLinkClass}
          >
            <RowText
              title={chat.title}
              meta={<ChatRowMeta chat={chat} titles={titles} />}
            />
            <Time iso={chat.last_turn_at} />
          </Link>
        )}
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
        renderRow={(observation) => {
          const messages = messageCountText(observation.message_count)
          return (
            <Link {...observationLink(observation.id)} className={rowLinkClass}>
              <IconTile>
                <NotebookTextIcon />
              </IconTile>
              <RowText
                title={plainTitle(observation.title)}
                meta={
                  <>
                    <span>Started by {observation.created_by.label}</span>
                    <RelativeTime iso={observation.created_at} />
                    {messages === null ? null : <span>{messages}</span>}
                  </>
                }
              />
              <AuthorMark mark={markOf(observation.created_by)} size={20} />
            </Link>
          )
        }}
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
          const meta = importMeta(entry.counts)
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
                    {meta.status === null ? null : (
                      <StatusLabel tone={meta.status.tone}>
                        {meta.status.text}
                      </StatusLabel>
                    )}
                    <span>{meta.counts}</span>
                  </>
                }
              />
              <Time iso={entry.created_at} />
              <AuthorMark mark={markOfAuthor(entry.created_by)} size={20} />
            </Link>
          )
        }}
      />
    </HomeSection>
  )
}
