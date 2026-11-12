import {
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react"
import { useNavigate, useSearch } from "@tanstack/react-router"
import {
  ArrowLeftIcon,
  ChartLineIcon,
  CompassIcon,
  DownloadIcon,
  EllipsisIcon,
  ExpandIcon,
  ImageIcon,
  InfoIcon,
  Maximize2Icon,
  MessageSquarePlusIcon,
  NotebookTextIcon,
  PencilIcon,
  PinIcon,
  SearchIcon,
  TableIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { focusMovedOutOfMenu } from "@/lib/focus"
import { isEditablePanel } from "@/panel-editor/draft"
import { useAgentInfo } from "@/observe/queries"
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import { panelUsesVariables } from "@/observations/panel-input"
import { isRenderable } from "@/panels"
import { shortcutText } from "@/shortcuts/chords"
import {
  isDataPanel,
  panelTitle,
  resolveDatasourceRef,
  type LayoutPanel,
} from "@/spec"

import { exploreProblemText, exploreTarget } from "./explore-link"
import { askAboutPanel, commentOnPanel, editPanelQuery } from "./panel-actions"
import { useSetDashboardSearch, validateDashboardSearch } from "./search"

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
  inToolbar?: boolean
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
  inToolbar = false,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const navigate = useNavigate()
  const rawSearch = useSearch({ strict: false })
  const dashboardSearch = useMemo(
    () => validateDashboardSearch(rawSearch),
    [rawSearch]
  )
  const datasources = useDatasources().data
  const agentAvailable = useAgentInfo().data?.available === true
  const menuButton = useRef<HTMLButtonElement>(null)
  const [addingToObservation, setAddingToObservation] = useState(false)
  const items: MenuItem[] = []
  if (canComment) {
    items.push({
      id: "comment",
      label: idle ? "Comment" : "Comment (finish the layout first)",
      icon: <MessageSquarePlusIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        focusAfterClose = "composer"
        commentOnPanel(setSearch, panel.id)
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
        askAboutPanel(setSearch, panel.id)
      },
    })
  }
  if (data && idle) {
    const usesVariables = panelUsesVariables(panel)
    items.push({
      id: "observation",
      label: usesVariables
        ? "Add to observation (uses dashboard variables)"
        : "Add to observation",
      icon: <NotebookTextIcon aria-hidden="true" />,
      disabled: usesVariables,
      onSelect: () => {
        focusAfterClose = "none"
        setAddingToObservation(true)
      },
    })
  }
  const canPropose = canComment && !canEdit
  if ((canEdit || canPropose) && isEditablePanel(panel)) {
    const name = canEdit ? "Edit" : "Suggest an edit"
    items.push({
      id: "edit",
      label: idle ? name : `${name} (finish the layout first)`,
      icon: <PencilIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        focusAfterClose = "none"
        editPanelQuery(setSearch, panel.id)
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
  if (data && (panel.type === "logs" || panel.type === "traces")) {
    const explorer = panel.type === "logs" ? "Open in Logs" : "Open in Traces"
    const explore = exploreTarget(
      panel,
      (ref) => resolveDatasourceRef(ref, dashboardSearch.vars),
      (uid) => datasourceTypeOf(datasources, uid),
      { from: dashboardSearch.from, to: dashboardSearch.to }
    )
    items.push({
      id: "explore",
      label:
        "problem" in explore
          ? `${explorer} (${exploreProblemText(explore.problem)})`
          : explorer,
      icon: <CompassIcon aria-hidden="true" />,
      disabled: "problem" in explore,
      onSelect: () => {
        if ("problem" in explore) return
        focusAfterClose = "none"
        void navigate(explore.target)
      },
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
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            inToolbar ? (
              <Button
                ref={menuButton}
                variant="ghost"
                size="icon-sm"
                aria-label={`More actions for ${panelTitle(panel)}`}
              />
            ) : (
              <Button
                ref={menuButton}
                variant="ghost"
                className="size-[26px] text-muted-foreground"
                aria-label={`Panel menu: ${panelTitle(panel)}`}
                data-panel-menu={panel.id}
              />
            )
          }
        >
          <EllipsisIcon
            aria-hidden="true"
            className={inToolbar ? "size-4" : "size-3.5"}
          />
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
      {addingToObservation && data ? (
        <AddToObservationPanel
          panel={panel}
          from={dashboardSearch.from}
          to={dashboardSearch.to}
          trigger={menuButton}
          onClose={() => setAddingToObservation(false)}
        />
      ) : null}
    </>
  )
}
