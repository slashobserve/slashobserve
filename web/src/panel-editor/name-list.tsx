import { SearchIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

export const nameSearchDelayMs = 250
export const listClass = "m-0 max-h-64 list-none overflow-y-auto py-1"
const rowClass =
  "block w-full truncate px-2.5 py-[5px] text-left font-mono hover:bg-selection focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
const skeletonRows = [0, 1, 2]

export function ListState({
  items,
  isError,
  onRetry,
  empty,
}: {
  items: unknown[] | undefined
  isError: boolean
  onRetry: () => void
  empty: string
}) {
  if (isError) {
    return (
      <li className="flex flex-col items-start gap-1.5 px-2.5 py-1.5">
        <span className="text-muted-foreground">Could not load names.</span>
        <Button type="button" variant="outline" size="xs" onClick={onRetry}>
          Retry
        </Button>
      </li>
    )
  }
  if (items === undefined) {
    return skeletonRows.map((row) => (
      <li key={row} className="px-2.5 py-[5px]">
        <Skeleton className="h-[18px]" />
      </li>
    ))
  }
  if (items.length === 0) {
    return <li className="px-2.5 py-[5px] text-muted-foreground">{empty}</li>
  }
  return null
}

export function TruncatedNote({ show }: { show: boolean }) {
  if (!show) return null
  return (
    <p className="m-0 border-t border-border px-2.5 py-1.5 text-muted-foreground">
      Showing the first 100. Type to narrow.
    </p>
  )
}

export function NameRow({
  text,
  onClick,
  pressed,
  title,
  note,
}: {
  text: string
  onClick: () => void
  pressed?: boolean
  title?: string
  note?: string
}) {
  return (
    <li>
      <button
        type="button"
        className={cn(rowClass, pressed && "bg-selection")}
        aria-pressed={pressed}
        title={title}
        onClick={onClick}
      >
        {text}
        {note ? (
          <span className="ml-2 font-sans text-muted-foreground">{note}</span>
        ) : null}
      </button>
    </li>
  )
}

export function SearchBox({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex items-center gap-2 border-b border-border px-2.5 py-2">
      <SearchIcon
        aria-hidden="true"
        className="size-[13px] shrink-0 text-muted-foreground"
      />
      <input
        aria-label={label}
        className="min-w-0 flex-1 rounded-control border-0 bg-transparent font-mono text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  )
}
