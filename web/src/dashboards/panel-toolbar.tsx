import {
  useLayoutEffect,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react"
import {
  AtSignIcon,
  MessageSquarePlusIcon,
  NotebookTextIcon,
  SquarePenIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import { panelUsesVariables } from "@/observations/panel-input"
import { isEditablePanel } from "@/panel-editor/draft"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { askAboutPanel, commentOnPanel, editPanelQuery } from "./panel-actions"
import type { DashboardPermissions } from "./permissions"
import { useSetDashboardSearch } from "./search"

const flipBelowWithin = 48

type Props = {
  panel: LayoutPanel
  permissions: Pick<DashboardPermissions, "comment" | "edit">
  composerRef: RefObject<HTMLTextAreaElement | null>
  from: string | undefined
  to: string | undefined
  idle: boolean
  menu: ReactNode
}

function focusComposerWhenShown(
  composerRef: RefObject<HTMLTextAreaElement | null>
) {
  requestAnimationFrame(() => composerRef.current?.focus())
}

function enabledButtons(root: HTMLElement): HTMLButtonElement[] {
  return Array.from(root.querySelectorAll<HTMLButtonElement>("button")).filter(
    (button) => !button.disabled
  )
}

function useFlipBelow(ref: RefObject<HTMLElement | null>): boolean {
  const [below, setBelow] = useState(false)
  useLayoutEffect(() => {
    const panel = ref.current?.parentElement
    if (!panel) return
    const container = panel.closest("main") ?? document.documentElement
    const measure = () =>
      setBelow(
        panel.getBoundingClientRect().top -
          container.getBoundingClientRect().top <
          flipBelowWithin
      )
    measure()
    container.addEventListener("scroll", measure, { passive: true })
    window.addEventListener("resize", measure)
    const observer = new ResizeObserver(measure)
    observer.observe(panel)
    return () => {
      container.removeEventListener("scroll", measure)
      window.removeEventListener("resize", measure)
      observer.disconnect()
    }
  }, [ref])
  return below
}

function ToolbarButton({
  label,
  tooltip = label,
  disabled,
  onClick,
  children,
  buttonRef,
}: {
  label: string
  tooltip?: string
  disabled?: boolean
  onClick: () => void
  children: ReactNode
  buttonRef?: RefObject<HTMLButtonElement | null>
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            ref={buttonRef}
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            disabled={disabled}
            onClick={onClick}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  )
}

export function PanelToolbar({
  panel,
  permissions,
  composerRef,
  from,
  to,
  idle,
  menu,
}: Props) {
  const root = useRef<HTMLDivElement>(null)
  const observationButton = useRef<HTMLButtonElement>(null)
  const [stop, setStop] = useState(0)
  const [addingToObservation, setAddingToObservation] = useState(false)
  const setSearch = useSetDashboardSearch()
  const agentAvailable = useAgentInfo().data?.available === true
  const below = useFlipBelow(root)
  const title = panelTitle(panel)
  const data = isDataPanel(panel)
  const canPropose = permissions.comment && !permissions.edit
  const usesVariables = data && panelUsesVariables(panel)

  useLayoutEffect(() => {
    if (!root.current) return
    const buttons = enabledButtons(root.current)
    const current = Math.min(stop, buttons.length - 1)
    buttons.forEach((button, index) => {
      button.tabIndex = index === current ? 0 : -1
    })
  })

  function onFocus(event: FocusEvent<HTMLDivElement>) {
    const index = enabledButtons(event.currentTarget).findIndex(
      (button) => button === document.activeElement
    )
    if (index >= 0) setStop(index)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const buttons = enabledButtons(event.currentTarget)
    const index = buttons.findIndex(
      (button) => button === document.activeElement
    )
    if (index < 0) return
    let next: number
    switch (event.key) {
      case "ArrowRight":
        next = Math.min(index + 1, buttons.length - 1)
        break
      case "ArrowLeft":
        next = Math.max(index - 1, 0)
        break
      case "Home":
        next = 0
        break
      case "End":
        next = buttons.length - 1
        break
      default:
        return
    }
    event.preventDefault()
    buttons[next].focus()
  }

  const finishFirst = (name: string) =>
    idle ? name : `${name} (finish the layout first)`

  return (
    <div
      ref={root}
      role="toolbar"
      aria-label={`Actions for ${title}`}
      onFocus={onFocus}
      onKeyDown={onKeyDown}
      className={cn(
        "absolute right-0 z-20 flex items-center gap-0.5 rounded-control border border-border bg-popover p-0.5 text-popover-foreground",
        below ? "-bottom-10" : "-top-10"
      )}
    >
      {permissions.comment ? (
        <ToolbarButton
          label="Comment"
          tooltip={finishFirst("Comment")}
          disabled={!idle}
          onClick={() => {
            commentOnPanel(setSearch, panel.id)
            focusComposerWhenShown(composerRef)
          }}
        >
          <MessageSquarePlusIcon aria-hidden="true" />
        </ToolbarButton>
      ) : null}
      {permissions.edit && data && agentAvailable ? (
        <ToolbarButton
          label="Ask @observe"
          tooltip={finishFirst("Ask @observe")}
          disabled={!idle}
          onClick={() => {
            askAboutPanel(setSearch, panel.id)
            focusComposerWhenShown(composerRef)
          }}
        >
          <AtSignIcon aria-hidden="true" />
        </ToolbarButton>
      ) : null}
      {(permissions.edit || canPropose) && isEditablePanel(panel) ? (
        <ToolbarButton
          label={permissions.edit ? "Edit query" : "Suggest an edit"}
          tooltip={finishFirst(
            permissions.edit ? "Edit query" : "Suggest an edit"
          )}
          disabled={!idle}
          onClick={() => editPanelQuery(setSearch, panel.id)}
        >
          <SquarePenIcon aria-hidden="true" />
        </ToolbarButton>
      ) : null}
      {data ? (
        <ToolbarButton
          label="Add to observation"
          tooltip={
            usesVariables
              ? "Add to observation (uses dashboard variables)"
              : "Add to observation"
          }
          disabled={!idle || usesVariables}
          buttonRef={observationButton}
          onClick={() => setAddingToObservation(true)}
        >
          <NotebookTextIcon aria-hidden="true" />
        </ToolbarButton>
      ) : null}
      {menu}
      {addingToObservation && data ? (
        <AddToObservationPanel
          panel={panel}
          from={from}
          to={to}
          trigger={observationButton}
          onClose={() => setAddingToObservation(false)}
        />
      ) : null}
    </div>
  )
}
