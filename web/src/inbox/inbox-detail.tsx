import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { proposalPath } from "@/lib/links"
import { cn } from "@/lib/utils"
import { sentence } from "@/observe/queries"
import { ProposalActions } from "@/proposals/proposal-actions"
import { ProposalBody } from "@/proposals/proposal-body"
import type { Picks } from "@/proposals/proposal-view"
import { proposalQuery } from "@/proposals/queries"

import {
  itemExcerpt,
  itemLink,
  itemText,
  threadTarget,
  type InboxItem,
} from "./inbox-item"
import { ReasonChip } from "./reason-chip"
import { ThreadDetail } from "./thread-detail"

const linkClass = "w-fit text-[13px] underline underline-offset-4"

function ProposalDetail({ proposalId }: { proposalId: string }) {
  const query = useQuery(proposalQuery(proposalId))
  const [picks, setPicks] = useState<Picks>({})
  const [merging, setMerging] = useState(false)
  const proposal = query.data

  if (proposal !== undefined) {
    return (
      <div className="flex flex-col gap-4">
        <ProposalBody
          proposal={proposal}
          picks={picks}
          onPick={(key, side) =>
            setPicks((current) => ({ ...current, [key]: side }))
          }
          disabled={merging}
          headingLevel={2}
        />
        <ProposalActions
          proposal={proposal}
          picks={picks}
          onPicksChange={setPicks}
          onMergingChange={setMerging}
        />
        <Link to={proposalPath(proposal.id)} className={linkClass}>
          Open proposal
        </Link>
      </div>
    )
  }
  if (query.error instanceof ApiError && query.error.status === 404) {
    return (
      <p className="text-[13px] text-muted-foreground">
        This proposal does not exist or you cannot see it.
      </p>
    )
  }
  if (query.isError) {
    return (
      <LoadProblem
        title="Could not load this proposal"
        detail={sentence(query.error.message)}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
      />
    )
  }
  return (
    <div aria-busy="true" className="flex flex-col gap-3">
      <Skeleton className="h-6 w-1/2" />
      <Skeleton className="h-10 rounded-card" />
      <Skeleton className="h-10 rounded-card" />
    </div>
  )
}

function ItemDetail({
  item,
  done,
  onToggleUnread,
  onToggleDone,
}: {
  item: InboxItem
  done: boolean
  onToggleUnread: (item: InboxItem) => void
  onToggleDone: (item: InboxItem) => void
}) {
  const { actor, verb, object } = itemText(item)
  const link = itemLink(item)
  const excerpt = itemExcerpt(item)
  const target = threadTarget(item)
  return (
    <div className="flex flex-col gap-3">
      <h2 className="text-lg font-semibold wrap-anywhere">
        {actor === "" ? null : `${actor} `}
        {verb} {object}
      </h2>
      <p className="flex items-center gap-2 text-[13px] text-muted-foreground">
        {item.reason === null ? null : <ReasonChip reason={item.reason} />}
        <RelativeTime iso={item.created_at} />
      </p>
      {target !== null ? (
        <ThreadDetail
          key={item.target.id}
          target={target}
          threadId={item.target.id}
        />
      ) : excerpt === null ? null : (
        <p className="text-[13px] wrap-anywhere">{excerpt}</p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {link === null ? null : (
          <Link
            to={link.to}
            search={link.search}
            className={cn(
              buttonVariants({ variant: "outline" }),
              "w-fit no-underline"
            )}
          >
            Open
          </Link>
        )}
        <Button
          type="button"
          variant="ghost"
          onClick={() => onToggleUnread(item)}
        >
          {item.unread ? "Mark as read" : "Mark as unread"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={() => onToggleDone(item)}
        >
          {done ? "Move back to the inbox" : "Mark done"}
        </Button>
      </div>
    </div>
  )
}

export function InboxDetail({
  item,
  done,
  onToggleUnread,
  onToggleDone,
}: {
  item: InboxItem | null
  done: boolean
  onToggleUnread: (item: InboxItem) => void
  onToggleDone: (item: InboxItem) => void
}) {
  if (item === null) {
    return (
      <p className="text-[13px] text-muted-foreground">
        Select an item to see it here.
      </p>
    )
  }
  if (item.target.kind === "proposal") {
    return <ProposalDetail key={item.target.id} proposalId={item.target.id} />
  }
  return (
    <ItemDetail
      item={item}
      done={done}
      onToggleUnread={onToggleUnread}
      onToggleDone={onToggleDone}
    />
  )
}
