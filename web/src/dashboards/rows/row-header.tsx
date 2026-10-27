import { ChevronDownIcon } from "lucide-react"

import { cn } from "@/lib/utils"

type Props = {
  rowId: string
  title: string
  collapsed: boolean
  childCount: number
  onToggle: (() => void) | undefined
}

export function RowHeader({
  rowId,
  title,
  collapsed,
  childCount,
  onToggle,
}: Props) {
  return (
    <div
      data-row-id={rowId}
      data-collapsed={collapsed}
      className="flex size-full items-center border-b px-1"
    >
      <h2 className="min-w-0 text-sm font-semibold">
        <button
          type="button"
          aria-expanded={!collapsed}
          aria-label={`${collapsed ? "Expand" : "Collapse"} row ${title}`}
          disabled={onToggle === undefined}
          onClick={onToggle}
          className="flex max-w-full items-center gap-1.5 rounded-control px-1 py-0.5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <ChevronDownIcon
            aria-hidden="true"
            className={cn("size-3.5 shrink-0", collapsed && "-rotate-90")}
          />
          <span className="truncate">{title}</span>
          {collapsed ? (
            <span className="shrink-0 text-xs font-normal text-muted-foreground">
              {childCount === 1 ? "1 panel" : `${childCount} panels`}
            </span>
          ) : null}
        </button>
      </h2>
    </div>
  )
}
