import type { ReactNode } from "react"
import {
  ChartLineIcon,
  EllipsisIcon,
  MessageSquarePlusIcon,
  PencilIcon,
  TableIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { panelRenderers } from "@/panels"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { useSetDashboardSearch } from "./search"

type Props = {
  panel: LayoutPanel
  tableView: boolean
  onToggleTableView: () => void
  inspectable: boolean
}

type MenuItem = {
  id: string
  label: string
  icon: ReactNode
  onSelect: () => void
  disabled?: boolean
}

export function PanelMenu({
  panel,
  tableView,
  onToggleTableView,
  inspectable,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const items: MenuItem[] = [
    {
      id: "comment",
      label: inspectable ? "Comment" : "Comment (finish the layout first)",
      icon: <MessageSquarePlusIcon aria-hidden="true" />,
      disabled: !inspectable,
      onSelect: () =>
        setSearch((state) => ({ ...state, panel: panel.id, tab: "comments" })),
    },
  ]
  const data = isDataPanel(panel)
  if (data) {
    items.push({
      id: "edit",
      label: inspectable ? "Edit" : "Edit (finish the layout first)",
      icon: <PencilIcon aria-hidden="true" />,
      disabled: !inspectable,
      onSelect: () =>
        setSearch((state) => ({ ...state, panel: panel.id, tab: "query" })),
    })
  }
  if (data && panel.type !== "table" && panelRenderers[panel.type]) {
    items.push({
      id: "table-view",
      label: tableView ? "View as chart" : "View as table",
      icon: tableView ? (
        <ChartLineIcon aria-hidden="true" />
      ) : (
        <TableIcon aria-hidden="true" />
      ),
      onSelect: onToggleTableView,
    })
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            className="size-[26px] text-muted-foreground"
            aria-label={`Panel menu: ${panelTitle(panel)}`}
            data-panel-menu={panel.id}
          />
        }
      >
        <EllipsisIcon aria-hidden="true" className="size-3.5" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-max">
        {items.map((item) => (
          <DropdownMenuItem
            key={item.id}
            disabled={item.disabled}
            onClick={item.onSelect}
          >
            {item.icon}
            {item.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
