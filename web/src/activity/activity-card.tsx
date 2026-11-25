import {
  useCallback,
  useId,
  useMemo,
  useRef,
  type Ref,
  type RefObject,
} from "react"

import type { Selections } from "@/api/keys"
import { DashboardChat, type OpenChat } from "@/chat/dashboard-chat"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { useDockFocus } from "@/components/dock/dock-focus"
import { activityCardId, paneOpener } from "@/dashboards/pane-openers"
import {
  useDashboardSearch,
  useSetDashboardSearch,
  type SidePanelTab,
} from "@/dashboards/search"
import type { EditorView } from "@/panel-editor/panel-editor"
import { SharePane } from "@/share/share-pane"
import { findPanel, type Dashboard } from "@/spec"
import type { SidePanelCard } from "@/dashboards/dock"

import { ActivityFeed } from "./activity-feed"
import {
  useActivityLayout,
  useActivityStore,
  useCurrentChat,
} from "./activity-store"
import { EditPane, type EditGuard } from "./edit-pane"
import { SidePanelTabs } from "./side-panel-tabs"

type Props = {
  view: SidePanelCard
  hidden?: boolean
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
  view: cardView,
  hidden = false,
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
  const chatId = useCurrentChat(dashboardId, search.chat)
  const setWidth = useActivityStore((state) => state.setWidth)
  const setEditWidth = useActivityStore((state) => state.setEditWidth)
  const headingId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const card = useRef<HTMLElement>(null)

  const panelId = search.panel
  const editingId = canEdit && cardView === "edit" ? panelId : undefined
  const editing = editingId !== undefined
  const sharing = cardView === "share"
  const tab: SidePanelTab = cardView === "chat" ? "chat" : "activity"
  const view: EditorView = search.edit === "json" ? "json" : "editor"
  const layout = useActivityLayout(editing)

  const openerOfPane = useCallback(
    () => paneOpener(sharing ? "share" : "activity"),
    [sharing]
  )
  const { close } = useDockFocus({
    pane: card,
    heading,
    opener: openerOfPane,
    onClose,
    closeOnEscape: !editing,
    active: !hidden,
    modeKey: `${panelId}|${editing}|${sharing}|${tab}`,
  })

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

  const chatPanelId = tab === "chat" ? panelId : undefined
  const chatPanelTitle =
    chatPanelId === undefined
      ? undefined
      : findPanel(spec, chatPanelId)?.panel.title
  const chatPanel = useMemo(
    () =>
      chatPanelId === undefined || chatPanelTitle === undefined
        ? null
        : {
            id: chatPanelId,
            title: chatPanelTitle,
            onRemove: () => onScopeChange(null),
          },
    [chatPanelId, chatPanelTitle, onScopeChange]
  )

  const onBack = useCallback(() => {
    setOpen(true)
    setSearch((state) => ({ ...state, edit: undefined, panel: undefined }))
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

  const onTabChange = useCallback(
    (next: SidePanelTab) => {
      setOpen(true)
      setSearch((state) => ({ ...state, tab: next }), { replace: true })
    },
    [setOpen, setSearch]
  )

  const onOpenChat = useCallback<OpenChat>(
    (chat, options) =>
      setSearch((state) => ({ ...state, tab: "chat", chat }), options),
    [setSearch]
  )

  return (
    <DockPanel
      id={activityCardId}
      ref={card}
      labelledBy={headingId}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={editing ? setEditWidth : setWidth}
      stacked={editing && layout.stacked ? "sheet" : layout.stacked}
      hidden={hidden}
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
            onClose={close}
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
        <SidePanelTabs
          headingId={headingId}
          headingRef={heading}
          tab={tab}
          onTabChange={onTabChange}
          onClose={close}
        >
          {tab === "chat" ? (
            <DashboardChat
              dashboardId={apiId}
              dashboardTitle={spec.title}
              chatId={chatId}
              onOpenChat={onOpenChat}
              panel={chatPanel}
            />
          ) : (
            <ActivityFeed
              dashboardId={dashboardId}
              apiId={apiId}
              spec={spec}
              pinned={pinned}
              layoutEditing={false}
              madeBy={madeBy}
              scopePanelId={panelId ?? null}
              onScopeChange={onScopeChange}
              onSelectPanel={onSelectPanel}
              back={back}
              composerRef={composerRef}
              focusedThreadId={search.thread}
            />
          )}
        </SidePanelTabs>
      )}
    </DockPanel>
  )
}
