import { MessageSquareIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { useSetDashboardSearch } from "@/dashboards/search"

type Props = {
  panelId: string
  title: string
  count: number
  disabled?: boolean
}

export function PanelCommentBadge({ panelId, title, count, disabled }: Props) {
  const setSearch = useSetDashboardSearch()
  if (count === 0) return null
  return (
    <Button
      variant="ghost"
      className="h-[26px] gap-1 rounded-control bg-muted px-[7px] text-xs"
      disabled={disabled}
      aria-label={`${count} open ${count === 1 ? "comment" : "comments"} on ${title}`}
      onClick={() =>
        setSearch((state) => ({ ...state, panel: panelId, tab: "comments" }))
      }
    >
      <MessageSquareIcon aria-hidden="true" className="size-3" />
      <span className="tabular-nums">{count}</span>
    </Button>
  )
}
