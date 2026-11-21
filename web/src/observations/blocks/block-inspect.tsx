import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react"
import { createPortal } from "react-dom"

import type { InspectTab } from "@/dashboards/search"
import { InspectPane, type InspectTarget } from "@/inspect/inspect-pane"
import { displayZone } from "@/lib/time"
import type { Block } from "@/observations/queries"
import { PanelHostContext } from "@/shell/side-panel"
import { isDataPanel, panelTitle, type LayoutPanel } from "@/spec"

import { panelBlockBody } from "./block-data"

export type BlockInspect = {
  inspectedId: string | null
  open: (blockId: string) => void
  publish: (target: InspectTarget | null) => void
}

export const BlockInspectContext = createContext<BlockInspect | null>(null)

export function useBlockInspect(): BlockInspect | null {
  return useContext(BlockInspectContext)
}

type Inspecting = { blockId: string; tab: InspectTab }

export function useBlockInspectState(blocks: readonly Block[]) {
  const [inspecting, setInspecting] = useState<Inspecting | null>(null)
  const [target, setTarget] = useState<InspectTarget | null>(null)
  const block =
    inspecting === null
      ? undefined
      : blocks.find(
          (candidate) =>
            candidate.id === inspecting.blockId && candidate.kind === "panel"
        )
  const inspectedId = block?.id ?? null
  const open = useCallback(
    (blockId: string) => setInspecting({ blockId, tab: "data" }),
    []
  )
  const context = useMemo<BlockInspect>(
    () => ({ inspectedId, open, publish: setTarget }),
    [inspectedId, open]
  )
  const setTab = useCallback(
    (tab: InspectTab) =>
      setInspecting((current) =>
        current === null ? null : { ...current, tab }
      ),
    []
  )
  const close = useCallback(() => setInspecting(null), [])
  return {
    context,
    block,
    tab: inspecting?.tab ?? "data",
    target,
    setTab,
    close,
  }
}

export function BlockInspectDock({
  block,
  tab,
  target,
  onTabChange,
  onClose,
}: {
  block: Block
  tab: InspectTab
  target: InspectTarget | null
  onTabChange: (tab: InspectTab) => void
  onClose: () => void
}) {
  const host = useContext(PanelHostContext)
  const body = useMemo(() => panelBlockBody(block), [block])
  if (host === null || body === null) return null
  return createPortal(
    <InspectPane
      title={panelTitle(body.panel as LayoutPanel)}
      tab={tab}
      target={target}
      variables={body.variables ?? []}
      zone={displayZone(
        isDataPanel(body.panel) ? body.panel.timezone : undefined,
        undefined
      )}
      onClose={onClose}
      onTabChange={onTabChange}
      inContainer
    />,
    host
  )
}
