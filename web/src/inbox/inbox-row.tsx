import { Link } from "@tanstack/react-router"

import { AuthorMark } from "@/components/author-mark"
import { RelativeTime } from "@/components/relative-time"
import { Badge } from "@/components/ui/badge"
import { markOf } from "@/lib/author"
import { cn } from "@/lib/utils"

import { itemHref, itemText, reasonLabel, type InboxItem } from "./inbox-item"

type Props = {
  item: InboxItem
  selected: boolean
  tabStop: boolean
  asLink: boolean
  onSelect: (item: InboxItem) => void
}

const rowClass =
  "flex min-h-14 w-full items-center gap-3 rounded-control px-3 py-2 text-left no-underline outline-none focus-visible:ring-2 focus-visible:ring-ring"

function ItemLine({ item }: { item: InboxItem }) {
  const { actor, verb, object } = itemText(item)
  return (
    <span className="line-clamp-2 text-[13px] leading-snug">
      {actor === "" ? null : `${actor} `}
      {verb} <span className="font-semibold">{object}</span>
    </span>
  )
}

function RowBody({ item }: { item: InboxItem }) {
  return (
    <>
      <AuthorMark mark={markOf(item.actor)} />
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        <ItemLine item={item} />
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          {item.reason === null ? null : (
            <Badge variant="outline">{reasonLabel(item.reason)}</Badge>
          )}
          <RelativeTime iso={item.created_at} />
        </span>
      </span>
      {item.unread ? (
        <>
          <span
            aria-hidden="true"
            className="size-[7px] flex-none rounded-full bg-foreground"
          />
          <span className="sr-only">Unread</span>
        </>
      ) : null}
    </>
  )
}

export function InboxRow({ item, selected, tabStop, asLink, onSelect }: Props) {
  const className = cn(rowClass, selected && "bg-selection")
  const href = itemHref(item)
  if (asLink && href !== null) {
    return (
      <Link
        to={href}
        data-inbox-row=""
        tabIndex={tabStop ? 0 : -1}
        className={className}
      >
        <RowBody item={item} />
      </Link>
    )
  }
  return (
    <button
      type="button"
      data-inbox-row=""
      tabIndex={tabStop ? 0 : -1}
      aria-current={selected ? "true" : undefined}
      onClick={() => onSelect(item)}
      className={className}
    >
      <RowBody item={item} />
    </button>
  )
}
