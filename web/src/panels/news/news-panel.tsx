import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { ExternalLinkIcon } from "lucide-react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { newsKeys } from "@/api/keys"
import { RelativeTime } from "@/components/relative-time"

import { errorMessage, PanelErrorState } from "../panel-error"
import { PanelSkeleton } from "../panel-skeleton"
import type { StaticPanelRenderer } from "../types"
import { isNotAllowed, newsHref, retryNews } from "./news-model"

const fiveMinutes = 5 * 60_000

function Message({ children }: { children: React.ReactNode }) {
  return (
    <div className="m-0 flex size-full flex-col items-center justify-center gap-1 p-3 text-center text-xs text-muted-foreground">
      {children}
    </div>
  )
}

const linkClass =
  "rounded-sm font-medium wrap-anywhere outline-none hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"

const NewsPanel: StaticPanelRenderer = ({ panel }) => {
  const feedUrl = panel.type === "news" ? panel.display?.feedUrl : undefined
  const feed = useQuery({
    queryKey: newsKeys.feed(feedUrl ?? ""),
    queryFn: ({ signal }) =>
      api
        .GET("/news", { params: { query: { url: feedUrl ?? "" } }, signal })
        .then(unwrap),
    enabled: feedUrl !== undefined && feedUrl !== "",
    staleTime: fiveMinutes,
    retry: retryNews,
  })

  if (feedUrl === undefined || feedUrl === "") {
    return <Message>Set a feed URL in the panel settings.</Message>
  }
  if (feed.isPending) return <PanelSkeleton />
  if (feed.isError) {
    if (isNotAllowed(feed.error)) {
      return (
        <Message>
          <p className="m-0">{errorMessage(feed.error)}</p>
          <Link
            to="/settings"
            hash="outbound-access"
            className="rounded-sm font-medium text-foreground underline underline-offset-2 outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Allow it in Settings
          </Link>
        </Message>
      )
    }
    return (
      <PanelErrorState
        title="Feed unavailable"
        error={feed.error}
        onRetry={() => void feed.refetch()}
      />
    )
  }
  const { items } = feed.data
  if (items.length === 0) return <Message>The feed has no items.</Message>
  return (
    <ul
      data-news
      className="m-0 size-full list-none overflow-auto p-0 pb-2 text-xs"
    >
      {items.map((item, index) => {
        const href = newsHref(item.link)
        return (
          <li
            key={`${index}-${item.link ?? item.title}`}
            className="flex min-w-0 flex-col gap-0.5 px-3 py-1.5"
          >
            <div className="flex min-w-0 flex-wrap items-baseline gap-x-2">
              {href === null ? (
                <span className="font-medium wrap-anywhere">{item.title}</span>
              ) : (
                <a
                  href={href}
                  target="_blank"
                  rel="noopener noreferrer nofollow"
                  className={`${linkClass} text-[13px] no-underline`}
                >
                  {item.title}
                  <ExternalLinkIcon
                    aria-hidden="true"
                    className="ml-0.5 inline size-3 align-baseline"
                  />
                </a>
              )}
              {item.published === null ? null : (
                <span className="text-muted-foreground">
                  <RelativeTime iso={item.published} />
                </span>
              )}
            </div>
            {item.summary === "" ? null : (
              <p className="m-0 line-clamp-2 text-muted-foreground">
                {item.summary}
              </p>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export default NewsPanel
