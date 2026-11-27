import {
  useCallback,
  useId,
  useMemo,
  useState,
  type ReactNode,
  type RefObject,
} from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"

import type { components } from "@/api/schema.gen"
import { RequestCommentAccess } from "@/access/request-comment-access"
import { RoleNote } from "@/components/role-note"
import { ScrollPane } from "@/components/scroll-pane"
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
  useLatestEditNote,
  useLatestPermissions,
  useLatestProvisioned,
} from "@/dashboards/permissions"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { useVersionHistory } from "@/history/use-version-history"
import { importForDashboardQuery } from "@/import/record-queries"
import { annotationActivityQuery } from "@/annotations/queries"
import { findingsForDashboardQuery } from "@/findings/queries"
import { dashboardProposalsQuery } from "@/proposals/queries"
import { fixPrompt } from "@/import/record-summary"
import { useAgentInfo, useMyRequests } from "@/observe/queries"
import { isActive, useRevisions } from "@/revisions/queries"
import { allPanels, panelTitle, type Dashboard } from "@/spec"

import { threadParticipants } from "@/comments/mentions"
import { useThreads } from "@/comments/thread-queries"
import { deliveryGate, type GateInput } from "@/conversation/delivery"
import { ThreadView } from "@/conversation/thread-view"

import {
  ActivityComposer,
  noPanelValue,
  panelItems,
  type Replying,
  type Target,
} from "./activity-composer"
import { useActivityStore, useDraft, useStoredTarget } from "./activity-store"
import { clientNameFrom } from "./client-name"
import { cn } from "@/lib/utils"
import { ExternalAgentNote } from "./external-agent-note"
import { AnnotationEntry } from "./annotation-entry"
import { FindingEntry } from "./finding-entry"
import { ProposalEntry } from "./proposal-entry"
import { ImportEntry } from "./import-entry"
import { RequestEntry } from "./request-entry"
import { RunningEntry } from "./running-entry"
import {
  ShowFilter,
  TimelineFilters,
  truncatingTriggerClass,
} from "./show-filter"
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
  pinned: boolean
  layoutEditing: boolean
  madeBy: string | null
  scopePanelId: string | null
  onScopeChange: (panelId: string | null) => void
  onSelectPanel: (panelId: string) => void
  back: string | undefined
  composerRef: RefObject<HTMLTextAreaElement | null>
  focusedThreadId?: string
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

function replyLabel(
  panelId: string | null,
  titles: ReadonlyMap<string, string>
): string {
  if (panelId === null) return "the dashboard"
  return titles.get(panelId) ?? "a removed panel"
}

function changeSignature(entry: TimelineEntry | undefined): string {
  if (entry === undefined) return ""
  switch (entry.kind) {
    case "thread":
      return `${entry.key}:${entry.thread.thread.status}:${entry.thread.messages.length}`
    case "request":
    case "running":
      return `${entry.key}:${entry.request.status}`
    case "version":
      return entry.key
    case "import":
      return `${entry.key}:${entry.unverified.length}`
    case "finding":
      return `${entry.key}:${entry.finding.annotation_id ?? ""}`
    case "proposal":
    case "annotation":
      return entry.key
  }
}

export function ActivityFeed({
  dashboardId,
  apiId,
  spec,
  pinned,
  layoutEditing,
  madeBy,
  scopePanelId,
  onScopeChange,
  onSelectPanel,
  back,
  composerRef,
  focusedThreadId,
}: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const revisions = useRevisions(dashboardId)
  const history = useVersionHistory(dashboardId)
  const agent = useAgentInfo()
  const panels = useLatestPanelRefs(dashboardId)
  const latestUnverified = useLatestUnverified(dashboardId)
  const importRecord = useQuery(importForDashboardQuery(dashboardId)).data
  const findingsData = useQuery(findingsForDashboardQuery(dashboardId)).data
  const annotationsData = useQuery(annotationActivityQuery(dashboardId)).data
  const openProposals = useQuery(dashboardProposalsQuery(dashboardId)).data
  const mergedProposals = useQuery(
    dashboardProposalsQuery(dashboardId, "merged")
  ).data
  const proposalsData = useMemo(
    () => [
      ...(openProposals?.proposals ?? []),
      ...(mergedProposals?.proposals ?? []),
    ],
    [openProposals, mergedProposals]
  )
  const activeRuns = useMyRequests({
    dashboardId,
    states: ["queued", "running", "canceling"],
  }).requests
  const threadTarget = `dashboard:${dashboardId}`
  const threadsQuery = useThreads(threadTarget)
  const threads = threadsQuery.threads
  const [replyingTo, setReplyingTo] = useState<string | null>(null)
  const investigations = useMemo(
    () => activeRuns.filter((request) => request.kind === "investigate"),
    [activeRuns]
  )
  const [show, setShow] = useState<TimelineShow>("everything")
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
          threads,
          requests,
          versions,
          imported,
          investigations,
          findings: findingsData,
          proposals: proposalsData,
          annotations: annotationsData,
        }),
        show,
        scopePanelId
      ),
    [
      threads,
      requests,
      versions,
      imported,
      investigations,
      findingsData,
      proposalsData,
      annotationsData,
      show,
      scopePanelId,
    ]
  )

  const permissions = useLatestPermissions(dashboardId)
  const folderTitle = useLatestFolderTitle(dashboardId)
  const provisioned = useLatestProvisioned(dashboardId)
  const mode = composerMode(permissions, provisioned)
  const note = readerNote(mode, folderTitle ?? "this folder")
  const editNote = useLatestEditNote(dashboardId)
  const gateInput: GateInput = {
    canComment: permissions.comment,
    agentAvailable: agent.data?.available,
    pinned,
    layoutEditing,
  }
  const gate = deliveryGate(gateInput, { to: "agent", visibleTo: "everyone" })
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
  const replied =
    replyingTo === null
      ? undefined
      : threads.find((entry) => entry.thread.id === replyingTo)
  const repliedThread = replied?.thread
  const replying: Replying | null =
    replyingTo === null
      ? null
      : {
          threadId: replyingTo,
          label: replyLabel(repliedThread?.panel_id ?? null, titles),
          agent: repliedThread?.agent ?? null,
          agentOff: repliedThread?.agent_off ?? false,
          people: threadParticipants(
            (replied?.messages ?? []).map((message) => message.actor)
          ),
        }
  const replyHere = useCallback(
    (threadId: string) => {
      setReplyingTo(threadId)
      requestAnimationFrame(() => composerRef.current?.focus())
    },
    [composerRef]
  )
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
          <ThreadView
            entry={entry.thread}
            target={threadTarget}
            focused={entry.thread.thread.id === focusedThreadId}
            panelTitle={title}
            onSelectPanel={onSelectPanel}
            gate={gate}
            clientName={clientName}
            onReplyHere={() => replyHere(entry.thread.thread.id)}
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
      case "proposal":
        return <ProposalEntry entry={entry} />
      case "annotation":
        return (
          <AnnotationEntry
            entry={entry}
            panelTitle={title}
            onSelectPanel={onSelectPanel}
          />
        )
    }
  }

  const noModel = gate.kind === "no-model"
  const filtered = show !== "everything" || scopePanelId !== null
  const scopeItems = panelItems("All panels", panelList)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TimelineFilters>
        <ShowFilter options={showOptions} value={show} onChange={setShow} />
        <label
          htmlFor={scopeId}
          className="shrink-0 text-xs text-muted-foreground"
        >
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
            className={cn("max-w-[200px]", truncatingTriggerClass)}
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
      </TimelineFilters>
      <ScrollPane ref={stickRef} className="flex-1">
        <TimelineList
          entries={entries}
          loading={threadsQuery.isPending}
          error={threads.length === 0 && threadsQuery.isError}
          onRetry={() => void threadsQuery.refetch()}
          filtered={filtered}
          older={{
            available: history.hasNextPage && showsVersions(show, scopePanelId),
            fetching: history.isFetchingNextPage,
            error: history.isFetchNextPageError ? history.error.message : null,
            onLoad: () => void history.fetchNextPage(),
          }}
          renderEntry={renderEntry}
        />
      </ScrollPane>
      {mode === "read-only" ? (
        <div className="flex flex-col gap-2 border-t px-4 py-3">
          <RoleNote>{note}</RoleNote>
          <RequestCommentAccess object={{ kind: "dashboard", id: apiId }} />
        </div>
      ) : (
        <ActivityComposer
          dashboardId={dashboardId}
          panels={panelList}
          target={target}
          onTargetChange={onTargetChange}
          replying={replying}
          onLeaveThread={() => setReplyingTo(null)}
          draft={draft}
          onDraftChange={onDraftChange}
          gate={gateInput}
          placeholder={
            noModel
              ? "Leave a comment for the team and Claude Code"
              : "Write a comment, or type @ to ask the agent"
          }
          textareaRef={composerRef}
          runningRequestId={runningRequestId}
          notice={
            mode === "comment-only" || mode === "provisioned" ? (
              <RoleNote>{note}</RoleNote>
            ) : (
              <>
                {editNote === null ? null : <RoleNote>{editNote}</RoleNote>}
                {noModel && madeBy !== null ? (
                  <ExternalAgentNote
                    dashboardId={apiId}
                    title={spec.title}
                    clientName={clientName}
                  />
                ) : null}
              </>
            )
          }
        />
      )}
    </div>
  )
}
