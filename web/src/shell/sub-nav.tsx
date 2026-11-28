import { useEffect, useState, type ReactNode } from "react"

import { Skeleton } from "@/components/ui/skeleton"

const rowsKey = "slashobserve-sidebar-rows"

function readRows(): Record<string, number> {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(rowsKey) ?? "{}")
    return typeof parsed === "object" && parsed !== null
      ? (parsed as Record<string, number>)
      : {}
  } catch {
    return {}
  }
}

export function storedRows(list: string): number {
  const count = readRows()[list]
  return count !== undefined && Number.isInteger(count) && count > 0 ? count : 0
}

export function storeRows(list: string, count: number) {
  try {
    localStorage.setItem(
      rowsKey,
      JSON.stringify({ ...readRows(), [list]: count })
    )
  } catch {
    return
  }
}

export function useReservedRows(
  list: string,
  pending: boolean,
  count: number
): number {
  const [reserved] = useState(() => storedRows(list))
  useEffect(() => {
    if (!pending) storeRows(list, count)
  }, [list, pending, count])
  return reserved
}

export const subNavLinkClass =
  "flex h-7 items-center gap-2 rounded-control border border-transparent px-2 text-[13px] text-foreground no-underline outline-none hover:bg-nav-active focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-nav-active"

export const subNavMoreClass =
  "flex h-7 items-center rounded-control px-2 text-xs text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"

export function SubNavList({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <ul
      aria-label={label}
      className="ml-[18px] flex flex-col gap-0.5 border-l pl-1.5"
    >
      {children}
    </ul>
  )
}

export function SubNavSkeleton({
  label,
  rows,
}: {
  label: string
  rows: number
}) {
  if (rows === 0) return null
  return (
    <SubNavList label={label}>
      {Array.from({ length: rows }, (_, index) => (
        <li
          key={index}
          aria-hidden="true"
          className="flex h-7 items-center px-2"
        >
          <Skeleton className="h-3 w-3/4" />
        </li>
      ))}
    </SubNavList>
  )
}
