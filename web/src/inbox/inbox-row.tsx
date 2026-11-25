import { Link } from "@tanstack/react-router"
import { CheckIcon, MailIcon, MailOpenIcon, UndoIcon } from "lucide-react"

import { AuthorMark } from "@/components/author-mark"
import { RelativeTime } from "@/components/relative-time"
import { Button } from "@/components/ui/button"
import { markOf } from "@/lib/author"
import { cn } from "@/lib/utils"

import {
  itemExcerpt,
  itemHint,
  itemLink,
  itemText,
  type InboxItem,
} from "./inbox-item"
import { ReasonChip } from "./reason-chip"

type Props = {
  item: InboxItem
  selected: boolean
  tabStop: boolean
  asLink: boolean
  onSelect: (item: InboxItem) => void
  onOpen?: (item: InboxItem) => void
  onToggleUnread?: (item: InboxItem) => void
  onToggleDone?: (item: InboxItem) => void
  done?: boolean
  expanded?: boolean
}

const rowClass =
  "flex min-h-14 w-full items-start gap-3 rounded-control py-2 pr-3 pl-3 text-left no-underline outline-none focus-visible:ring-2 focus-visible:ring-ring"

function ItemLine({ item, full }: { item: InboxItem; full: boolean }) {
  const { actor, verb, object } = itemText(item)
  return (
    <span
      className={cn(
        "text-[13px] leading-snug",
        !full && "line-clamp-2",
        item.unread && "font-medium"
      )}
    >
      {actor === "" ? null : `${actor} `}
      {verb} <span className="font-semibold">{object}</span>
    </span>
  )
}

function RowBody({ item, full }: { item: InboxItem; full: boolean }) {
  const excerpt = itemExcerpt(item)
  const hint = itemHint(item)
  return (
    <>
      <span
        aria-hidden="true"
        className={cn(
          "mt-[11px] size-[7px] flex-none rounded-full",
          item.unread ? "bg-primary" : "bg-transparent"
        )}
      />
      {item.unread ? <span className="sr-only">Unread</span> : null}
      <AuthorMark mark={markOf(item.actor)} />
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <ItemLine item={item} full={full} />
        {excerpt === null ? null : (
          <span
            className={cn(
              "text-xs leading-snug wrap-anywhere text-muted-foreground",
              !full && "line-clamp-2"
            )}
          >
            {excerpt}
          </span>
        )}
        {hint === null ? null : (
          <span
            className={cn(
              "text-xs leading-snug wrap-anywhere text-muted-foreground",
              !full && "line-clamp-3"
            )}
          >
            {hint}
          </span>
        )}
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          {item.reason === null ? null : <ReasonChip reason={item.reason} />}
          <RelativeTime iso={item.created_at} />
        </span>
      </span>
    </>
  )
}

export function InboxRow({
  item,
  selected,
  tabStop,
  asLink,
  onSelect,
  onOpen,
  onToggleUnread,
  onToggleDone,
  done = false,
  expanded,
}: Props) {
  const className = cn(
    rowClass,
    onToggleUnread !== undefined && "pr-10",
    onToggleUnread !== undefined && onToggleDone !== undefined && "pr-[4.5rem]",
    selected && "bg-selection"
  )
  const link = itemLink(item)
  const toggleLabel = item.unread ? "Mark as read" : "Mark as unread"
  const doneLabel = done ? "Move back to the inbox" : "Mark done"
  return (
    <div className="group/row relative">
      {asLink && link !== null ? (
        <Link
          to={link.to}
          search={link.search}
          data-inbox-row={item.seq}
          tabIndex={tabStop ? 0 : -1}
          aria-current={selected ? "true" : undefined}
          className={className}
          onClick={() => onOpen?.(item)}
        >
          <RowBody item={item} full={false} />
        </Link>
      ) : (
        <button
          type="button"
          data-inbox-row={item.seq}
          tabIndex={tabStop ? 0 : -1}
          aria-current={selected ? "true" : undefined}
          aria-expanded={expanded}
          onClick={() => onSelect(item)}
          className={className}
        >
          <RowBody item={item} full={expanded === true} />
        </button>
      )}
      {onToggleDone === undefined ? null : (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={doneLabel}
          title={doneLabel}
          className="absolute top-2 right-9 text-muted-foreground opacity-0 group-focus-within/row:opacity-100 group-hover/row:opacity-100 pointer-coarse:opacity-100"
          onClick={() => onToggleDone(item)}
        >
          {done ? (
            <UndoIcon aria-hidden="true" />
          ) : (
            <CheckIcon aria-hidden="true" />
          )}
        </Button>
      )}
      {onToggleUnread === undefined ? null : (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={toggleLabel}
          title={toggleLabel}
          className="absolute top-2 right-2 text-muted-foreground opacity-0 group-focus-within/row:opacity-100 group-hover/row:opacity-100 pointer-coarse:opacity-100"
          onClick={() => onToggleUnread(item)}
        >
          {item.unread ? (
            <MailOpenIcon aria-hidden="true" />
          ) : (
            <MailIcon aria-hidden="true" />
          )}
        </Button>
      )}
    </div>
  )
}
