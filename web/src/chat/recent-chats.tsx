import { useNavigate } from "@tanstack/react-router"
import { useId } from "react"

import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

import { ChatRowMeta } from "./chat-row-meta"
import { useChats } from "./chats-queries"

const recentLimit = 5

export function RecentChats() {
  const navigate = useNavigate()
  const headingId = useId()
  const { chats, query } = useChats({}, recentLimit)

  const failed = query.data === undefined && !query.isPending

  if (query.data !== undefined && chats.length === 0) return null

  return (
    <section aria-labelledby={headingId} className="mt-7 w-full max-w-[720px]">
      <h2
        id={headingId}
        className="px-0.5 pb-2 text-[13px] font-medium text-muted-foreground"
      >
        Recent chats
      </h2>
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
            : chats.map((chat) => {
                return (
                  <li key={chat.id} className="border-b last:border-b-0">
                    <button
                      type="button"
                      className="flex w-full flex-wrap items-center gap-x-3 gap-y-0.5 px-3.5 py-2.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
                      onClick={() =>
                        void navigate({
                          to: "/c/$requestId",
                          params: { requestId: chat.id },
                        })
                      }
                    >
                      <span className="min-w-0 flex-1 truncate text-[13px] narrow:basis-full">
                        {chat.title}
                      </span>
                      <span className="flex max-w-[260px] min-w-0 flex-none items-center gap-x-2 text-xs text-muted-foreground">
                        <ChatRowMeta chat={chat} />
                      </span>
                      <span className="w-20 flex-none text-right text-xs text-muted-foreground narrow:ml-auto narrow:w-auto">
                        <RelativeTime iso={chat.last_turn_at} />
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
