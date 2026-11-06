import type { ReactNode } from "react"

import { Button } from "@/components/ui/button"

type FilterToolbarProps = {
  children: ReactNode
  summary?: ReactNode
  onClear?: () => void
  label: string
}

export function FilterToolbar({
  children,
  summary,
  onClear,
  label,
}: FilterToolbarProps) {
  return (
    <div
      role="group"
      aria-label={label}
      className="flex flex-wrap items-center gap-2 pb-3"
    >
      {children}
      {onClear ? (
        <Button variant="ghost" size="sm" onClick={onClear}>
          Clear filters
        </Button>
      ) : null}
      {summary ? (
        <div className="ml-auto text-xs text-muted-foreground tabular-nums">
          {summary}
        </div>
      ) : null}
    </div>
  )
}
