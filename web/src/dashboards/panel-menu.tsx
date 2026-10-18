import { EllipsisVerticalIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Panel } from "@/spec"

type Props = {
  panel: Panel
  tableView: boolean
  onToggleTableView: () => void
}

type MenuItem = { id: string; label: string; onSelect: () => void }

export function PanelMenu({ panel, tableView, onToggleTableView }: Props) {
  const items: MenuItem[] = []
  if (panel.type !== "table") {
    items.push({
      id: "table-view",
      label: tableView ? "Show chart" : "Show table view",
      onSelect: onToggleTableView,
    })
  }
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
      <DropdownMenuContent align="end">
        {items.map((item) => (
          <DropdownMenuItem key={item.id} onClick={item.onSelect}>
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
