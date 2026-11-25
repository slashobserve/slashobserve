import type { ReactNode } from "react"

import { Skeleton } from "@/components/ui/skeleton"

export type SkeletonRow = { kind: "skeleton"; id: string }

export function skeletonRows(count: number): SkeletonRow[] {
  return Array.from({ length: count }, (_, index) => ({
    kind: "skeleton",
    id: `skeleton:${index}`,
  }))
}

export function dataCell<R extends { kind: string }, K extends R["kind"]>(
  kind: K,
  skeletonClassName: string,
  render: (row: Extract<R, { kind: K }>) => ReactNode
) {
  return (row: R): ReactNode =>
    row.kind === kind ? (
      render(row as Extract<R, { kind: K }>)
    ) : row.kind === "skeleton" ? (
      <Skeleton className={skeletonClassName} />
    ) : null
}
