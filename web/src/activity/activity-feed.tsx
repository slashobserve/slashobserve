import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useState,
  type ReactNode,
  type RefObject,
} from "react"
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"

import type { components } from "@/api/schema.gen"
import { commentsQuery, useComments } from "@/comments/queries"
import { RoleNote } from "@/components/role-note"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  composerMode,
  readerNote,
  useLatestFolderTitle,
  useLatestPermissions,
} from "@/dashboards/permissions"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { useVersionHistory } from "@/history/use-version-history"
import { importForDashboardQuery } from "@/import/record-queries"
import { findingsForDashboardQuery } from "@/findings/queries"
import { fixPrompt } from "@/import/record-summary"
import { useAgentInfo, useMyRequests } from "@/observe/queries"
import { isActive, useRevisions } from "@/revisions/queries"
import { allPanels, panelTitle, type Dashboard } from "@/spec"

import {
  ActivityComposer,
  noPanelValue,
  panelItems,
  type Target,
} from "./activity-composer"
import { sendToAgentState } from "./agent-gate"
import { useActivityStore, useDraft, useStoredTarget } from "./activity-store"
import { clientNameFrom } from "./client-name"
import { ExternalAgentNote } from "./external-agent-note"
import { FindingEntry } from "./finding-entry"
import { ImportEntry } from "./import-entry"
import { RequestEntry } from "./request-entry"
import { RunningEntry } from "./running-entry"
import { ThreadEntry } from "./thread-entry"
import {
  buildTimeline,
  filterTimeline,
  showsVersions,
  type TimelineEntry,
  type TimelineShow,
} from "./timeline-model"
import { TimelineList, useStickToBottom } from "./timeline-list"
import { VersionEntry } from "./version-entry"

const showOptions: { value: TimelineShow; label: string }[] = [
  { value: "everything", label: "Everything" },
  { value: "open", label: "Open comments" },
  { value: "changes", label: "Changes only" },
]

type Props = {
  dashboardId: string
  apiId: string
  spec: Dashboard
  latestVersion: number
  pinned: boolean
  layoutEditing: boolean
  madeBy: string | null
  scopePanelId: string | null
  onScopeChange: (panelId: string | null) => void
  onSelectPanel: (panelId: string) => void
  back: string | undefined
  composerRef: RefObject<HTMLTextAreaElement | null>
  range: { from: string; to: string }
}

type PanelRef = { id: string; title: string }

type UnverifiedRef = { panelId: string; reason: string }

function useLatestPanelRefs(dashboardId: string): PanelRef[] | undefined {
  const queryClient = useQueryClient()
  const select = useCallback(
    (envelope: { version: { spec: unknown } }): PanelRef[] =>
      allPanels(asDashboardSpec(envelope.version.spec)).map(({ panel }) => ({
        id: panel.id,
        title: panelTitle(panel),
      })),
    []
  )
  return useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    staleTime: Infinity,
    select,
  }).data
}

function useLatestUnverified(dashboardId: string): UnverifiedRef[] | undefined {
  const queryClient = useQueryClient()
  const select = useCallback(
    (envelope: {
      version: { unverified: components["schemas"]["Unverified"] }
    }): UnverifiedRef[] =>
      envelope.version.unverified.panels.map(({ panel_id, issues }) => ({
        panelId: panel_id,
        reason: issues[0]?.message ?? "",
      })),
    []
  )
  return useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    staleTime: Infinity,
    select,
  }).data
}

let feedDashboardId: string | null = null

function useResetAgentKindOnLeave(dashboardId: string) {
  const setAgentKind = useActivityStore((state) => state.setAgentKind)
  useEffect(() => {
    feedDashboardId = dashboardId
    return () => {
      feedDashboardId = null
      // StrictMode remounts a new feed once in development, so a remount of the same dashboard cancels the reset.
      queueMicrotask(() => {
        if (feedDashboardId !== dashboardId) setAgentKind("revise")
      })
    }
  }, [dashboardId, setAgentKind])
}

function changeSignature(entry: TimelineEntry | undefined): string {
  if (entry === undefined) return ""
  switch (entry.kind) {
    case "thread":
      return `${entry.key}:${entry.thread.root.status}:${entry.thread.replies.length}:${entry.request?.status ?? ""}`
    case "request":
    case "running":
      return `${entry.key}:${entry.request.status}`
    case "version":
      return entry.key
    case "import":
      return `${entry.key}:${entry.unverified.length}`
    case "finding":
      return `${entry.key}:${entry.finding.annotation_id ?? ""}`
  }
}

export function ActivityFeed({
  dashboardId,
  apiId,
  spec,
  latestVersion,
  pinned,
  layoutEditing,
  madeBy,
  scopePanelId,
  onScopeChange,
  onSelectPanel,
  back,
  composerRef,
  range,
}: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const comments = useComments(dashboardId)
  const commentsData = useInfiniteQuery(commentsQuery(dashboardId)).data
  const revisions = useRevisions(dashboardId)
  const history = useVersionHistory(dashboardId)
  const agent = useAgentInfo()
  const panels = useLatestPanelRefs(dashboardId)
  const latestUnverified = useLatestUnverified(dashboardId)
  const importRecord = useQuery(importForDashboardQuery(dashboardId)).data
  const findingsData = useQuery(findingsForDashboardQuery(dashboardId)).data
  const investigations = useMyRequests({
    source: "investigate",
    dashboardId,
  }).requests
  useResetAgentKindOnLeave(dashboardId)
  const [show, setShow] = useState<TimelineShow>("everything")
  const showId = useId()
  const scopeId = useId()

  const panelList = useMemo(() => panels ?? [], [panels])
  const titles = useMemo(
    () => new Map(panelList.map((panel) => [panel.id, panel.title])),
    [panelList]
  )
  const imported = useMemo(
    () =>
      importRecord
        ? {
            record: importRecord,
            unverified: (latestUnverified ?? []).map(({ panelId, reason }) => ({
              panelId,
              title: titles.get(panelId) ?? panelId,
              reason,
            })),
          }
        : null,
    [importRecord, latestUnverified, titles]
  )
  const requests = useMemo(() => revisions.data ?? [], [revisions.data])
  const versions = useMemo(
    () => history.data?.pages.flatMap((page) => page.versions) ?? [],
    [history.data]
  )
  const entries = useMemo(
    () =>
      filterTimeline(
        buildTimeline({
          comments: comments.comments,
          requests,
          versions,
          imported,
          investigations,
          findings: findingsData,
        }),
        show,
        scopePanelId
      ),
    [
      comments.comments,
      requests,
      versions,
      imported,
      investigations,
      findingsData,
      show,
      scopePanelId,
    ]
  )

  const permissions = useLatestPermissions(dashboardId)
  const folderTitle = useLatestFolderTitle(dashboardId)
  const mode = composerMode(permissions)
  const note = readerNote(mode, folderTitle ?? "this folder")
  const gate = sendToAgentState({
    canComment: permissions.comment,
    agentAvailable: agent.data?.available,
    pinned,
    layoutEditing,
    activeRequests: requests.filter((request) => isActive(request.status))
      .length,
  })
  const clientName = clientNameFrom(madeBy)
  const runningRequestId =
    requests.find(
      (request) =>
        isActive(request.status) && request.requested_by.id === principal.id
    )?.id ?? investigations.find((request) => isActive(request.status))?.id

  const draft = useDraft(dashboardId)
  const storedTarget = useStoredTarget(dashboardId)
  const setDraft = useActivityStore((state) => state.setDraft)
  const setTarget = useActivityStore((state) => state.setTarget)
  const onDraftChange = useCallback(
    (text: string) => setDraft(dashboardId, text),
    [setDraft, dashboardId]
  )
  const onTargetChange = useCallback(
    (next: Target) => {
      setTarget(dashboardId, next.kind === "panel" ? next.panelId : null)
      onScopeChange(null)
    },
    [setTarget, dashboardId, onScopeChange]
  )
  const onAskToFix = useCallback(
    (panelId: string, title: string) => {
      setDraft(dashboardId, fixPrompt(title, panelId))
      onTargetChange({ kind: "panel", panelId })
      requestAnimationFrame(() => {
        const textarea = composerRef.current
        if (textarea === null) return
        textarea.focus()
        textarea.setSelectionRange(textarea.value.length, textarea.value.length)
      })
    },
    [setDraft, dashboardId, onTargetChange, composerRef]
  )
  const panelId = scopePanelId ?? storedTarget
  const target: Target =
    panelId === null ? { kind: "dashboard" } : { kind: "panel", panelId }

  const stickRef = useStickToBottom(
    `${entries.length}:${changeSignature(entries.at(-1))}`
  )

  function renderEntry(entry: TimelineEntry): ReactNode {
    const title =
      entry.panelId === null ? null : (titles.get(entry.panelId) ?? null)
    switch (entry.kind) {
      case "thread":
        return (
          <ThreadEntry
            dashboardId={dashboardId}
            entry={entry}
            panelTitle={title}
            onSelectPanel={onSelectPanel}
            gate={gate}
            latestVersion={latestVersion}
            clientName={clientName}
          />
        )
      case "request":
        return (
          <RequestEntry
            dashboardId={dashboardId}
            entry={entry}
            panelTitle={title}
            onSelectPanel={onSelectPanel}
          />
        )
      case "running":
        return <RunningEntry entry={entry} />
      case "version":
        return (
          <VersionEntry dashboardId={dashboardId} entry={entry} back={back} />
        )
      case "import":
        return (
          <ImportEntry
            entry={entry}
            scopePanelId={scopePanelId}
            gate={gate}
            onSelectPanel={onSelectPanel}
            onAskToFix={onAskToFix}
          />
        )
      case "finding":
        return <FindingEntry entry={entry} />
    }
  }

  const noModel = gate.kind === "no-model"
  const filtered = show !== "everything" || scopePanelId !== null
  const scopeItems = panelItems("All panels", panelList)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b px-4 py-2">
        <label htmlFor={showId} className="text-xs text-muted-foreground">
          Show
        </label>
        <Select
          items={showOptions}
          value={show}
          onValueChange={(next) => {
            if (next !== null) setShow(next)
          }}
        >
          <SelectTrigger id={showId} size="sm" aria-label="Show">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {showOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <label htmlFor={scopeId} className="text-xs text-muted-foreground">
          Panel
        </label>
        <Select
          items={scopeItems}
          value={scopePanelId ?? noPanelValue}
          onValueChange={(next) => {
            if (next !== null)
              onScopeChange(next === noPanelValue ? null : next)
          }}
        >
          <SelectTrigger
            id={scopeId}
            size="sm"
            aria-label="Panel"
            className="max-w-[200px]"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {scopeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div ref={stickRef} className="min-h-0 flex-1 overflow-y-auto">
        <TimelineList
          entries={entries}
          loading={comments.isPending}
          error={commentsData === undefined && comments.isError}
          onRetry={() => void comments.refetch()}
          filtered={filtered}
          older={{
            available: history.hasNextPage && showsVersions(show, scopePanelId),
            fetching: history.isFetchingNextPage,
            error: history.isFetchNextPageError ? history.error.message : null,
            onLoad: () => void history.fetchNextPage(),
          }}
          renderEntry={renderEntry}
        />
      </div>
      {mode === "read-only" ? (
        <RoleNote className="border-t px-4 py-3">{note}</RoleNote>
      ) : (
        <ActivityComposer
          dashboardId={dashboardId}
          latestVersion={latestVersion}
          panels={panelList}
          target={target}
          onTargetChange={onTargetChange}
          draft={draft}
          onDraftChange={onDraftChange}
          gate={gate}
          canEdit={permissions.edit}
          placeholder={
            noModel
              ? "Leave a comment for the team and Claude Code"
              : "Write a comment, or tell the agent what to change"
          }
          textareaRef={composerRef}
          runningRequestId={runningRequestId}
          range={range}
          notice={
            mode === "comment-only" ? (
              <RoleNote>{note}</RoleNote>
            ) : noModel && madeBy !== null ? (
              <ExternalAgentNote
                dashboardId={apiId}
                title={spec.title}
                clientName={clientName}
              />
            ) : null
          }
        />
      )}
    </div>
  )
}
