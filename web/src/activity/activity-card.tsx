import {
  useCallback,
  useEffect,
  useId,
  useRef,
  type Ref,
  type RefObject,
} from "react"

import type { Selections } from "@/api/keys"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { useDashboardSearch, useSetDashboardSearch } from "@/dashboards/search"
import { useAgentInfo } from "@/observe/queries"
import type { EditorView } from "@/panel-editor/panel-editor"
import { SharePane } from "@/share/share-pane"
import type { Dashboard } from "@/spec"

import { ActivityFeed } from "./activity-feed"
import { useActivityLayout, useActivityStore } from "./activity-store"
import { clientNameFrom } from "./client-name"
import { EditPane, type EditGuard } from "./edit-pane"

export const activityCardId = "activity-card"

type Props = {
  dashboardId: string
  apiId: string
  spec: Dashboard
  version: number
  latestVersion: number
  pinned: boolean
  canEdit: boolean
  folder: { id: string; title: string; general: boolean; personal: boolean }
  canManage: boolean
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
  folder,
  canManage,
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
  const sharing = search.share === true && !editing
  const view: EditorView = search.edit === "json" ? "json" : "editor"
  const layout = useActivityLayout(editing)

  const shown = useRef<{
    panelId: string | undefined
    editing: boolean
    sharing: boolean
  }>({ panelId: undefined, editing: false, sharing: false })
  useEffect(() => {
    const before = shown.current
    shown.current = { panelId, editing, sharing }
    if (
      before.panelId === panelId &&
      before.editing === editing &&
      before.sharing === sharing
    ) {
      return
    }
    if (card.current?.contains(document.activeElement)) return
    heading.current?.focus()
  }, [panelId, editing, sharing])

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
    <DockPanel
      id={activityCardId}
      ref={card}
      labelledBy={headingId}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={editing ? setEditWidth : setWidth}
      stacked={layout.stacked}
    >
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
      ) : sharing ? (
        <>
          <DockHeader
            headingId={headingId}
            headingRef={heading}
            title="Share"
            description="Copy a link for people on this server."
            onClose={onClose}
            closeLabel="Close panel"
          />
          <SharePane
            dashboardId={dashboardId}
            spec={spec}
            search={search}
            folder={folder}
            canManage={canManage}
          />
        </>
      ) : (
        <>
          <DockHeader
            headingId={headingId}
            headingRef={heading}
            title="Activity"
            description={description}
            onClose={onClose}
            closeLabel="Close panel"
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
            range={{
              from: timeRange.from ?? spec.time.from,
              to: timeRange.to ?? spec.time.to,
            }}
          />
        </>
      )}
    </DockPanel>
  )
}
