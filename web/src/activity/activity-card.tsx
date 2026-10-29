import {
  useCallback,
  useEffect,
  useId,
  useRef,
  type CSSProperties,
  type Ref,
  type RefObject,
} from "react"

import type { Selections } from "@/api/keys"
import { useDashboardSearch, useSetDashboardSearch } from "@/dashboards/search"
import { useAgentInfo } from "@/observe/queries"
import { cn } from "@/lib/utils"
import type { EditorView } from "@/panel-editor/panel-editor"
import type { Dashboard } from "@/spec"

import { CardHeader } from "./card-header"
import { ActivityFeed } from "./activity-feed"
import { useActivityLayout, useActivityStore } from "./activity-store"
import { clientNameFrom } from "./client-name"
import { EditPane, type EditGuard } from "./edit-pane"
import { ResizeHandle } from "./resize-handle"

export const activityCardId = "activity-card"

type Props = {
  dashboardId: string
  apiId: string
  spec: Dashboard
  version: number
  latestVersion: number
  pinned: boolean
  canEdit: boolean
  madeBy: string | null
  back: string | undefined
  timeRange: { from?: string; to?: string }
  selections: Selections | null
  composerRef: RefObject<HTMLTextAreaElement | null>
  guardRef: Ref<EditGuard>
  onSelectPanel: (panelId: string) => void
  onClose: () => void
}

export function ActivityCard({
  dashboardId,
  apiId,
  spec,
  version,
  latestVersion,
  pinned,
  canEdit,
  madeBy,
  back,
  timeRange,
  selections,
  composerRef,
  guardRef,
  onSelectPanel,
  onClose,
}: Props) {
  const search = useDashboardSearch()
  const setSearch = useSetDashboardSearch()
  const setOpen = useActivityStore((state) => state.setOpen)
  const setWidth = useActivityStore((state) => state.setWidth)
  const setEditWidth = useActivityStore((state) => state.setEditWidth)
  const agent = useAgentInfo()
  const headingId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const card = useRef<HTMLElement>(null)

  const panelId = search.panel
  const editingId = canEdit && search.edit !== undefined ? panelId : undefined
  const editing = editingId !== undefined
  const view: EditorView = search.edit === "json" ? "json" : "editor"
  const layout = useActivityLayout(editing)

  const shown = useRef<{ panelId: string | undefined; editing: boolean }>({
    panelId: undefined,
    editing: false,
  })
  useEffect(() => {
    const before = shown.current
    shown.current = { panelId, editing }
    if (before.panelId === panelId && before.editing === editing) return
    if (card.current?.contains(document.activeElement)) return
    heading.current?.focus()
  }, [panelId, editing])

  const onScopeChange = useCallback(
    (next: string | null) =>
      setSearch(
        (state) => ({
          ...state,
          panel: next ?? undefined,
          edit: undefined,
        }),
        { replace: true }
      ),
    [setSearch]
  )

  const onBack = useCallback(() => {
    setOpen(true)
    setSearch((state) => ({ ...state, edit: undefined }))
  }, [setOpen, setSearch])

  const onViewChange = useCallback(
    (next: EditorView) =>
      setSearch(
        (state) => ({ ...state, edit: next === "json" ? "json" : "query" }),
        { replace: true }
      ),
    [setSearch]
  )

  const onSaved = useCallback(() => {
    setOpen(true)
    setSearch((state) => ({
      ...state,
      panel: undefined,
      edit: undefined,
      version: undefined,
    }))
  }, [setOpen, setSearch])

  const noModel = agent.data?.available === false
  const description = !noModel
    ? "Comments, agent requests and saved versions."
    : madeBy === null
      ? "Comments and saved versions."
      : `Comments and versions. ${clientNameFrom(madeBy)} made this dashboard.`

  return (
    <aside
      id={activityCardId}
      ref={card}
      aria-labelledby={headingId}
      style={{ "--activity-w": `${layout.width}px` } as CSSProperties}
      className={cn(
        "relative flex min-h-0 min-w-0 flex-col rounded-card border bg-background",
        layout.stacked
          ? "mr-2.5 mb-2.5 ml-4 h-[70svh] flex-[1_1_100%]"
          : "m-2 ml-0.5 flex-[0_0_var(--activity-w)]"
      )}
    >
      {layout.stacked ? null : (
        <ResizeHandle
          width={layout.width}
          bounds={layout.bounds}
          onChange={editing ? setEditWidth : setWidth}
        />
      )}
      {editingId !== undefined ? (
        <EditPane
          key={editingId}
          dashboardId={dashboardId}
          spec={spec}
          panelId={editingId}
          version={version}
          latestVersion={latestVersion}
          pinned={pinned}
          view={view}
          onViewChange={onViewChange}
          timeRange={timeRange}
          selections={selections}
          onBack={onBack}
          onClose={onClose}
          onSaved={onSaved}
          headingId={headingId}
          headingRef={heading}
          guardRef={guardRef}
        />
      ) : (
        <>
          <CardHeader
            headingId={headingId}
            headingRef={heading}
            title="Activity"
            description={description}
            close={onClose}
          />
          <ActivityFeed
            dashboardId={dashboardId}
            apiId={apiId}
            spec={spec}
            latestVersion={latestVersion}
            pinned={pinned}
            layoutEditing={false}
            madeBy={madeBy}
            scopePanelId={panelId ?? null}
            onScopeChange={onScopeChange}
            onSelectPanel={onSelectPanel}
            back={back}
            composerRef={composerRef}
          />
        </>
      )}
    </aside>
  )
}
