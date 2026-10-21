import type { ReactNode, RefObject } from "react"
import {
  ChartLineIcon,
  EllipsisIcon,
  MessageSquarePlusIcon,
  PencilIcon,
  TableIcon,
} from "lucide-react"

import { useActivityStore } from "@/activity/activity-store"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { isRenderable } from "@/panels"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { useSetDashboardSearch } from "./search"

type Props = {
  panel: LayoutPanel
  tableView: boolean
  onToggleTableView: () => void
  inspectable: boolean
  composerRef: RefObject<HTMLTextAreaElement | null>
}

// Only one panel menu is open at a time, and the choice must survive the render that closes it.
let focusAfterClose: "composer" | "none" | null = null

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
  composerRef,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const items: MenuItem[] = [
    {
      id: "comment",
      label: inspectable ? "Comment" : "Comment (finish the layout first)",
      icon: <MessageSquarePlusIcon aria-hidden="true" />,
      disabled: !inspectable,
      onSelect: () => {
        focusAfterClose = "composer"
        useActivityStore.getState().setOpen(true)
        setSearch((state) => ({ ...state, panel: panel.id, edit: undefined }))
      },
    },
  ]
  const data = isDataPanel(panel)
  if (data) {
    items.push({
      id: "edit",
      label: inspectable ? "Edit" : "Edit (finish the layout first)",
      icon: <PencilIcon aria-hidden="true" />,
      disabled: !inspectable,
      onSelect: () => {
        focusAfterClose = "none"
        useActivityStore.getState().setOpen(true)
        setSearch((state) => ({ ...state, panel: panel.id, edit: "query" }))
      },
    })
  }
  if (data && panel.type !== "table" && isRenderable(panel.type)) {
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
      <DropdownMenuContent
        align="end"
        className="min-w-max"
        finalFocus={() => {
          const next = focusAfterClose
          focusAfterClose = null
          if (next === "composer") return composerRef.current ?? true
          return next !== "none"
        }}
      >
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
