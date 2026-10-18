import { EllipsisVerticalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Panel } from "@/spec"

import { useSetDashboardSearch } from "./search"

type Props = {
  panel: Panel
  tableView: boolean
  onToggleTableView: () => void
  inspectable: boolean
}

type MenuItem = {
  id: string
  label: string
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
  const items: MenuItem[] = []
  if (panel.type !== "table") {
    items.push({
      id: "table-view",
      label: tableView ? "Show chart" : "Show table view",
      onSelect: onToggleTableView,
    })
  }
  items.push({
    id: "edit-query",
    label: inspectable ? "Edit query" : "Edit query (finish layout first)",
    disabled: !inspectable,
    onSelect: () =>
      setSearch((state) => ({ ...state, panel: panel.id, tab: "query" })),
  })
  items.push({
    id: "comments",
    label: inspectable ? "Comments" : "Comments (finish layout first)",
    disabled: !inspectable,
    onSelect: () =>
      setSearch((state) => ({ ...state, panel: panel.id, tab: "comments" })),
  })
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={`Panel menu: ${panel.title}`}
            data-panel-menu={panel.id}
          />
        }
      >
        <EllipsisVerticalIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-max">
        {items.map((item) => (
          <DropdownMenuItem
            key={item.id}
            disabled={item.disabled}
            onClick={item.onSelect}
          >
            {item.label}
          </DropdownMenuItem>
        ))}
        {items.length === 0 ? (
          <DropdownMenuItem disabled>No actions available</DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
