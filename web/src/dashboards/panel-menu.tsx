import { useRef, useState, type ReactNode, type RefObject } from "react"
import { useNavigate } from "@tanstack/react-router"
import {
  ArrowLeftIcon,
  AtSignIcon,
  ChartLineIcon,
  CompassIcon,
  CopyPlusIcon,
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
  TableIcon,
  Trash2Icon,
} from "lucide-react"

import type { Selections } from "@/api/keys"
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
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import { isRenderable } from "@/panels"
import { shortcutText } from "@/shortcuts/chords"
import {
  isDataPanel,
  panelTitle,
  resolveDatasourceRef,
  type LayoutPanel,
  type Variable,
} from "@/spec"

import { exploreProblemText, exploreTarget } from "./explore-link"
import {
  askAboutThisPanel,
  commentOnThisPanel,
  editPanelQuery,
  useConversationActions,
} from "./panel-actions"
import { PanelHoverActions } from "./panel-hover-actions"
import {
  RemovePanelDialog,
  type PanelLayoutActions,
} from "./panel-layout-actions"
import { useSetDashboardSearch } from "./search"

type Props = {
  panel: LayoutPanel
  range: { from: string; to: string }
  selections: Selections
  tableView: boolean
  onToggleTableView: () => void
  idle: boolean
  viewing: boolean
  onCanvas: boolean
  onDownloadCsv?: () => void
  onSavePng?: () => void
  downloadsReady?: boolean
  canComment: boolean
  canEdit: boolean
  canPropose: boolean
  onToggleResponsive?: () => void
  layoutActions?: PanelLayoutActions
  composerRef: RefObject<HTMLTextAreaElement | null>
  inToolbar?: boolean
  observationVariables?: readonly Variable[]
  block?: BlockMenuHost
}

// Only one panel menu is open at a time, and the choice must survive the render that closes it.
let focusAfterClose: "composer" | "none" | null = null

const personInput = ["pointerdown", "keydown"] as const

// The menu unmounts after its exit animation, which can end after the person has already clicked elsewhere.
function returnFocusToComposerUntilInput() {
  focusAfterClose = "composer"
  function stop() {
    if (focusAfterClose === "composer") focusAfterClose = null
    for (const type of personInput)
      document.removeEventListener(type, stop, true)
  }
  for (const type of personInput) document.addEventListener(type, stop, true)
}

export type MenuItem = {
  id: string
  label: string
  icon: ReactNode
  onSelect: () => void
  disabled?: boolean
  destructive?: boolean
  shortcut?: string
  opensPopup?: boolean
}

export type BlockMenuHost = {
  onInspect?: () => void
  actions: MenuItem[]
}

export function PanelMenu({
  panel,
  range,
  selections,
  tableView,
  onToggleTableView,
  idle,
  viewing,
  onCanvas,
  onDownloadCsv,
  onSavePng,
  downloadsReady = true,
  canComment,
  canEdit,
  canPropose,
  onToggleResponsive,
  layoutActions,
  composerRef,
  inToolbar = false,
  observationVariables,
  block,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const navigate = useNavigate()
  const datasources = useDatasources().data
  const conversation = useConversationActions(panel, canComment)
  const menuButton = useRef<HTMLButtonElement>(null)
  const [addingToObservation, setAddingToObservation] = useState(false)
  const [removing, setRemoving] = useState(false)
  const items: MenuItem[] = []
  const data = isDataPanel(panel)
  const onDashboard = block === undefined
  if (onDashboard && conversation.comment) {
    items.push({
      id: "comment",
      label: idle ? "Comment" : "Comment (finish the layout first)",
      icon: <MessageSquarePlusIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        const place = commentOnThisPanel(setSearch, panel.id, onCanvas)
        if (place === "pane") returnFocusToComposerUntilInput()
        else focusAfterClose = "none"
      },
      shortcut: shortcutText("comment"),
    })
  } else if (onDashboard) {
    items.push({
      id: "comment",
      label: "Comment (needs Comment access on this folder)",
      icon: <MessageSquarePlusIcon aria-hidden="true" />,
      disabled: true,
      onSelect: () => {},
    })
  }
  if (onDashboard && conversation.ask) {
    items.push({
      id: "ask",
      label: idle ? "Ask" : "Ask (finish the layout first)",
      icon: <AtSignIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        askAboutThisPanel(setSearch, panel.id, onCanvas)
        focusAfterClose = "none"
      },
      shortcut: shortcutText("ask"),
    })
  }
  if (onDashboard && data && idle) {
    items.push({
      id: "observation",
      label: "Add to observation",
      icon: <NotebookTextIcon aria-hidden="true" />,
      onSelect: () => {
        focusAfterClose = "none"
        setAddingToObservation(true)
      },
    })
  }
  if (onDashboard && (canEdit || canPropose) && isEditablePanel(panel)) {
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
  if (onDashboard) {
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
  }
  const inspectable = onDashboard || block.onInspect !== undefined
  if (data && inspectable && isRenderable(panel.type)) {
    items.push({
      id: "inspect",
      label: idle ? "Inspect" : "Inspect (finish the layout first)",
      icon: <InfoIcon aria-hidden="true" />,
      disabled: !idle,
      onSelect: () => {
        focusAfterClose = "none"
        if (block?.onInspect !== undefined) {
          block.onInspect()
          return
        }
        setSearch((state) => ({
          ...state,
          edit: undefined,
          share: false,
          inspect: panel.id,
          inspectTab: undefined,
        }))
      },
      shortcut: onDashboard ? shortcutText("inspect") : undefined,
    })
  }
  if (data && (panel.type === "logs" || panel.type === "traces")) {
    const explorer = panel.type === "logs" ? "Open in Logs" : "Open in Traces"
    const explore = exploreTarget(
      panel,
      (ref) => resolveDatasourceRef(ref, selections),
      (uid) => datasourceTypeOf(datasources, uid),
      range
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
      disabled: !downloadsReady,
      onSelect: onDownloadCsv,
    })
  }
  if (onSavePng) {
    items.push({
      id: "png",
      label: "Save as PNG",
      icon: <ImageIcon aria-hidden="true" />,
      disabled: !downloadsReady,
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
  if (layoutActions) {
    items.push(
      {
        id: "duplicate",
        label: "Duplicate",
        icon: <CopyPlusIcon aria-hidden="true" />,
        disabled: layoutActions.pending,
        onSelect: () => layoutActions.duplicate(panel),
      },
      {
        id: "remove",
        label: "Remove",
        icon: <Trash2Icon aria-hidden="true" />,
        disabled: layoutActions.pending,
        destructive: true,
        onSelect: () => {
          focusAfterClose = "none"
          setRemoving(true)
        },
      }
    )
  }
  if (block) items.push(...block.actions)
  return (
    <>
      {onDashboard && !inToolbar ? (
        <PanelHoverActions
          panel={panel}
          canComment={canComment}
          idle={idle}
          viewing={viewing}
          onCanvas={onCanvas}
          composerRef={composerRef}
        />
      ) : null}
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
              variant={item.destructive ? "destructive" : undefined}
              onClick={() => {
                if (item.opensPopup) focusAfterClose = "none"
                item.onSelect()
              }}
            >
              {item.icon}
              {item.label}
              {item.shortcut ? (
                <DropdownMenuShortcut
                  aria-hidden="true"
                  className="max-sm:hidden pointer-coarse:hidden"
                >
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
          variables={observationVariables}
          from={range.from}
          to={range.to}
          trigger={menuButton}
          onClose={() => setAddingToObservation(false)}
        />
      ) : null}
      {removing && layoutActions ? (
        <RemovePanelDialog
          panel={panel}
          actions={layoutActions}
          onClose={() => setRemoving(false)}
        />
      ) : null}
    </>
  )
}
