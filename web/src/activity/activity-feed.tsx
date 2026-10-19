import { useCallback, useMemo, useState, type ReactNode, type Ref } from "react"
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { commentsQuery, useComments } from "@/comments/queries"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { useVersionHistory } from "@/history/use-version-history"
import { useAgentInfo } from "@/observe/queries"
import { isActive, useRevisions } from "@/revisions/queries"
import { allPanels, panelTitle, type Dashboard } from "@/spec"

import { ActivityComposer, type Target } from "./activity-composer"
import { sendToAgentState } from "./agent-gate"
import { useActivityStore, useDraft, useStoredTarget } from "./activity-store"
import { clientNameFrom } from "./client-name"
import { ExternalAgentNote } from "./external-agent-note"
import { RequestEntry } from "./request-entry"
import { RunningEntry } from "./running-entry"
import { ThreadEntry } from "./thread-entry"
import {
  buildTimeline,
  filterTimeline,
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

const selectClass =
  "h-[26px] min-w-0 max-w-[200px] rounded-[5px] border bg-background px-1.5 text-xs text-foreground"

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
  composerRef: Ref<HTMLTextAreaElement>
}

type PanelRef = { id: string; title: string }

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
  }
}

function FilterSelect({
  label,
  value,
  onChange,
  children,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  children: ReactNode
}) {
  return (
    <label className="flex min-w-0 items-center gap-1.5">
      {label}
      <select
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className={selectClass}
      >
        {children}
      </select>
    </label>
  )
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
}: Props) {
  const comments = useComments(dashboardId)
  const commentsData = useInfiniteQuery(commentsQuery(dashboardId)).data
  const revisions = useRevisions(dashboardId)
  const history = useVersionHistory(dashboardId)
  const agent = useAgentInfo()
  const panels = useLatestPanelRefs(dashboardId)
  const [show, setShow] = useState<TimelineShow>("everything")

  const panelList = useMemo(() => panels ?? [], [panels])
  const titles = useMemo(
    () => new Map(panelList.map((panel) => [panel.id, panel.title])),
    [panelList]
  )
  const requests = useMemo(() => revisions.data ?? [], [revisions.data])
  const versions = useMemo(
    () => history.data?.pages[0]?.versions ?? [],
    [history.data]
  )
  const entries = useMemo(
    () =>
      filterTimeline(
        buildTimeline({
          comments: comments.comments,
          requests,
          versions,
        }),
        show,
        scopePanelId
      ),
    [comments.comments, requests, versions, show, scopePanelId]
  )

  const gate = sendToAgentState({
    agentAvailable: agent.data?.available,
    pinned,
    layoutEditing,
    activeRequests: requests.filter((request) => isActive(request.status))
      .length,
  })
  const clientName = clientNameFrom(madeBy)

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
    }
  }

  const noModel = gate.kind === "no-model"
  const filtered = show !== "everything" || scopePanelId !== null

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center justify-between gap-2 px-4 py-2.5 text-xs text-muted-foreground">
        <FilterSelect
          label="Show"
          value={show}
          onChange={(value) =>
            setShow(
              showOptions.find((option) => option.value === value)?.value ??
                "everything"
            )
          }
        >
          {showOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </FilterSelect>
        <FilterSelect
          label="Panel"
          value={scopePanelId ?? ""}
          onChange={(value) => onScopeChange(value === "" ? null : value)}
        >
          <option value="">All panels</option>
          {panelList.map((panel) => (
            <option key={panel.id} value={panel.id}>
              {panel.title === "" ? "Untitled panel" : panel.title}
            </option>
          ))}
        </FilterSelect>
      </div>
      <div ref={stickRef} className="min-h-0 flex-1 overflow-y-auto">
        <TimelineList
          entries={entries}
          loading={comments.isPending}
          error={commentsData === undefined && comments.isError}
          onRetry={() => void comments.refetch()}
          filtered={filtered}
          renderEntry={renderEntry}
        />
      </div>
      <ActivityComposer
        dashboardId={dashboardId}
        latestVersion={latestVersion}
        panels={panelList}
        target={target}
        onTargetChange={onTargetChange}
        draft={draft}
        onDraftChange={onDraftChange}
        gate={gate}
        placeholder={
          noModel
            ? "Leave a comment for the team and Claude Code"
            : "Write a comment, or tell the agent what to change"
        }
        textareaRef={composerRef}
        notice={
          noModel && madeBy !== null ? (
            <ExternalAgentNote
              dashboardId={apiId}
              title={spec.title}
              clientName={clientName}
            />
          ) : null
        }
      />
    </div>
  )
}
