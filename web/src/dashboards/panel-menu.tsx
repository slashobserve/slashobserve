import type { ReactNode, RefObject } from "react"
import {
  ArrowLeftIcon,
  ChartLineIcon,
  DownloadIcon,
  EllipsisIcon,
  ExpandIcon,
  ImageIcon,
  InfoIcon,
  Maximize2Icon,
  MessageSquarePlusIcon,
  PencilIcon,
  PinIcon,
  SearchIcon,
  TableIcon,
} from "lucide-react"

import { useActivityStore } from "@/activity/activity-store"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { focusMovedOutOfMenu } from "@/lib/focus"
import { isEditablePanel } from "@/panel-editor/draft"
import { useAgentInfo } from "@/observe/queries"
import { isRenderable } from "@/panels"
import { shortcutText } from "@/shortcuts/chords"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { useSetDashboardSearch } from "./search"

type Props = {
  panel: LayoutPanel
  tableView: boolean
  onToggleTableView: () => void
  idle: boolean
  viewing: boolean
  onDownloadCsv?: () => void
  onSavePng?: () => void
  canComment: boolean
  canEdit: boolean
  onToggleResponsive?: () => void
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
  shortcut?: string
}

export function PanelMenu({
  panel,
  tableView,
  onToggleTableView,
  idle,
  viewing,
  onDownloadCsv,
  onSavePng,
  canComment,
  canEdit,
  onToggleResponsive,
  composerRef,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const agentAvailable = useAgentInfo().data?.available === true
  const items: MenuItem[] = []
  if (canComment) {
    items.push({
      id: "comment",
      label: idle ? "Comment" : "Comment (finish the layout first)",
      icon: <MessageSquarePlusIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        focusAfterClose = "composer"
        useActivityStore.getState().setOpen(true)
        setSearch((state) => ({ ...state, panel: panel.id, edit: undefined }))
      },
    })
  }
  const data = isDataPanel(panel)
  if (canEdit && data && agentAvailable) {
    items.push({
      id: "investigate",
      label: idle ? "Investigate" : "Investigate (finish the layout first)",
      icon: <SearchIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        focusAfterClose = "composer"
        const activity = useActivityStore.getState()
        activity.setOpen(true)
        activity.setAgentKind("investigate")
        setSearch((state) => ({ ...state, panel: panel.id, edit: undefined }))
      },
    })
  }
  if (canEdit && isEditablePanel(panel)) {
    items.push({
      id: "edit",
      label: idle ? "Edit" : "Edit (finish the layout first)",
      icon: <PencilIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        focusAfterClose = "none"
        useActivityStore.getState().setOpen(true)
        setSearch((state) => ({ ...state, panel: panel.id, edit: "query" }))
      },
      shortcut: shortcutText("edit"),
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
  items.push(
    viewing
      ? {
          id: "view",
          label: "Back to dashboard",
          icon: <ArrowLeftIcon aria-hidden="true" />,
          onSelect: () =>
            setSearch((state) => ({ ...state, viewPanel: undefined })),
          shortcut: shortcutText("view"),
        }
      : {
          id: "view",
          label: idle ? "View" : "View (finish the layout first)",
          icon: <ExpandIcon aria-hidden="true" />,
          disabled: !idle,
          onSelect: () =>
            setSearch((state) => ({ ...state, viewPanel: panel.id })),
          shortcut: shortcutText("view"),
        }
  )
  if (data && isRenderable(panel.type)) {
    items.push({
      id: "inspect",
      label: idle ? "Inspect" : "Inspect (finish the layout first)",
      icon: <InfoIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        focusAfterClose = "none"
        setSearch((state) => ({
          ...state,
          edit: undefined,
          share: false,
          inspect: panel.id,
          inspectTab: undefined,
        }))
      },
      shortcut: shortcutText("inspect"),
    })
  }
  if (onDownloadCsv) {
    items.push({
      id: "csv",
      label: "Download CSV",
      icon: <DownloadIcon aria-hidden="true" />,
      onSelect: onDownloadCsv,
    })
  }
  if (onSavePng) {
    items.push({
      id: "png",
      label: "Save as PNG",
      icon: <ImageIcon aria-hidden="true" />,
      onSelect: onSavePng,
    })
  }
  if (onToggleResponsive) {
    const fixed = panel.responsive === false
    items.push({
      id: "responsive",
      label: fixed ? "Make responsive" : "Keep this size",
      icon: fixed ? (
        <Maximize2Icon aria-hidden="true" />
      ) : (
        <PinIcon aria-hidden="true" />
      ),
      onSelect: onToggleResponsive,
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
          if (next === "none") return false
          return !focusMovedOutOfMenu(document.activeElement, document.body)
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
            {item.shortcut ? (
              <DropdownMenuShortcut aria-hidden="true">
                {item.shortcut}
              </DropdownMenuShortcut>
            ) : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
