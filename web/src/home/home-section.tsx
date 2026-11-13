import { useId, type ComponentProps, type ReactNode } from "react"
import { Link } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

type Props = {
  title: string
  action?: ReactNode
  children: ReactNode
}

type ListQuery = {
  isError: boolean
  isFetching: boolean
  fetchStatus: "fetching" | "paused" | "idle"
  refetch: () => unknown
}

type ListProps<T> = {
  items: readonly T[] | undefined
  query?: ListQuery
  getKey: (item: T) => string
  renderRow: (item: T) => ReactNode
  emptyText: string
  errorText?: string
}

export const textLinkClass =
  "rounded-control text-[13px] text-foreground underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"

export const rowLinkClass =
  "flex w-full items-center gap-3 px-4 py-2.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring narrow:min-h-11"

const offlineText =
  "You are offline. This list loads when the connection is back."

export function HomeSection({ title, action, children }: Props) {
  const id = useId()
  return (
    <section aria-labelledby={id} className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between gap-3">
        <h2 id={id} className="text-sm font-semibold">
          {title}
        </h2>
        {action}
      </div>
      {children}
    </section>
  )
}

export function SectionLink({
  className,
  ...props
}: ComponentProps<typeof Link>) {
  return <Link className={cn(textLinkClass, className)} {...props} />
}

function Note({ children }: { children: ReactNode }) {
  return <div className="rounded-card border bg-card px-4 py-3">{children}</div>
}

export function SectionList<T>({
  items,
  query,
  getKey,
  renderRow,
  emptyText,
  errorText = "",
}: ListProps<T>) {
  if (items === undefined && query?.fetchStatus === "paused") {
    return (
      <Note>
        <p className="text-[13px] text-muted-foreground">{offlineText}</p>
      </Note>
    )
  }
  if (items === undefined && query?.isError) {
    return (
      <Note>
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-[13px] text-muted-foreground">{errorText}</p>
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
      </Note>
    )
  }
  if (items === undefined) {
    return (
      <ul
        aria-busy="true"
        className="overflow-hidden rounded-card border bg-card"
      >
        {Array.from({ length: 3 }, (_, index) => (
          <li key={index} className="border-b last:border-b-0">
            <Skeleton className="h-[39px] rounded-none" />
          </li>
        ))}
      </ul>
    )
  }
  if (items.length === 0) {
    return (
      <Note>
        <p className="text-[13px] text-muted-foreground">{emptyText}</p>
      </Note>
    )
  }
  return (
    <ul className="overflow-hidden rounded-card border bg-card">
      {items.map((item) => (
        <li key={getKey(item)} className="border-b last:border-b-0">
          {renderRow(item)}
        </li>
      ))}
    </ul>
  )
}

export function RowText({ title, meta }: { title: string; meta: ReactNode }) {
  return (
    <span className="min-w-0 flex-1">
      <span className="block truncate text-[13px] font-medium">{title}</span>
      <span className="flex flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
        {meta}
      </span>
    </span>
  )
}

export function IconTile({ children }: { children: ReactNode }) {
  return (
    <span
      aria-hidden="true"
      className="grid size-7 flex-none place-items-center rounded-control border text-muted-foreground [&_svg]:size-3.5"
    >
      {children}
    </span>
  )
}
