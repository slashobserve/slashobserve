import { useEffect, type RefObject } from "react"
import {
  AtSignIcon,
  MessageSquarePlusIcon,
  MousePointer2Icon,
} from "lucide-react"
import { create } from "zustand"

import { Button } from "@/components/ui/button"
import { useAgentInfo } from "@/observe/queries"

import { askAboutDashboard } from "./panel-actions"
import type { DashboardPermissions } from "./permissions"
import { useSetDashboardSearch } from "./search"

type Tool = "select" | "comment"

type CanvasToolState = {
  tool: Tool
  setTool: (tool: Tool) => void
}

export const useCanvasTool = create<CanvasToolState>()((set) => ({
  tool: "select",
  setTool: (tool) => set({ tool }),
}))

type Props = {
  permissions: Pick<DashboardPermissions, "comment" | "edit">
  canInvestigate: boolean
  idle: boolean
  composerRef: RefObject<HTMLTextAreaElement | null>
}

const toolClass =
  "gap-1.5 text-[13px] aria-pressed:bg-selection aria-pressed:text-foreground"

export function useCanvasToolsShown(
  permissions: Pick<DashboardPermissions, "comment" | "edit">
): boolean {
  const agentAvailable = useAgentInfo().data?.available === true
  return permissions.comment || (permissions.edit && agentAvailable)
}

export function CanvasTools({
  permissions,
  canInvestigate,
  idle,
  composerRef,
}: Props) {
  const tool = useCanvasTool((state) => state.tool)
  const setTool = useCanvasTool((state) => state.setTool)
  const setSearch = useSetDashboardSearch()
  const agentAvailable = useAgentInfo().data?.available === true
  const mayAsk = permissions.edit && agentAvailable
  const commenting = permissions.comment && idle && tool === "comment"

  useEffect(() => () => setTool("select"), [setTool])

  if (!permissions.comment && !mayAsk) return null

  return (
    <div className="sticky bottom-0 z-30 h-0 narrow:hidden">
      <div
        role="toolbar"
        aria-label="Canvas tools"
        className="absolute bottom-4 left-1/2 flex h-10 -translate-x-1/2 items-center gap-1 rounded-control border border-border bg-popover px-1.5 text-popover-foreground"
      >
        <Button
          variant="ghost"
          size="sm"
          className={toolClass}
          aria-pressed={!commenting}
          onClick={() => setTool("select")}
        >
          <MousePointer2Icon aria-hidden="true" />
          Select
        </Button>
        {permissions.comment ? (
          <Button
            variant="ghost"
            size="sm"
            className={toolClass}
            aria-pressed={commenting}
            disabled={!idle}
            onClick={() => setTool("comment")}
          >
            <MessageSquarePlusIcon aria-hidden="true" />
            Comment
          </Button>
        ) : null}
        {mayAsk ? (
          <Button
            variant="ghost"
            size="sm"
            className={toolClass}
            disabled={!idle || !canInvestigate}
            focusableWhenDisabled={!canInvestigate}
            title={
              canInvestigate
                ? undefined
                : "Asking the agent needs the Editor role"
            }
            onClick={() => {
              setTool("select")
              askAboutDashboard(setSearch)
              requestAnimationFrame(() => composerRef.current?.focus())
            }}
          >
            <AtSignIcon aria-hidden="true" />
            {canInvestigate ? "Ask" : "Ask (needs the Editor role)"}
          </Button>
        ) : null}
      </div>
    </div>
  )
}
