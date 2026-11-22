import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type Ref,
} from "react"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { Skeleton } from "@/components/ui/skeleton"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  deletedText,
  useDeletedBy,
  useDeletedDashboards,
} from "@/dashboards/live/deleted-store"
import {
  contentChange,
  useLatestPermissions,
  useLatestProvisioned,
} from "@/dashboards/permissions"
import { usePatchPanels } from "@/dashboards/queries"
import { saveToast } from "@/dashboards/save-outcome"
import { useSetDashboardSearch } from "@/dashboards/search"
import type { Selections } from "@/api/keys"
import { DockHeader } from "@/components/dock/dock-header"
import { Button } from "@/components/ui/button"
import { proposalLink } from "@/lib/links"
import { guardLoad } from "@/panels/renderer-load"
import type {
  DashboardTarget,
  EditorView,
  PanelsSave,
} from "@/panel-editor/panel-editor"
import { isEditablePanel } from "@/panel-editor/draft"
import { useDraftTitle } from "@/panel-editor/draft-title"
import { useLeaveGuard } from "@/panel-editor/leave-guard"
import { useNewPanels, withNewPanel } from "@/panel-editor/new-panel"
import { findPanel, panelTitle, type Dashboard, type Panel } from "@/spec"

import { EditorBoundary } from "./editor-boundary"
import { useEditorEntryInDocument } from "./editor-entry"
import { EditorViewSwitch } from "./editor-view-switch"
import { leavesDraft } from "./leaves-draft"

const PanelEditor = lazy(guardLoad(() => import("@/panel-editor/panel-editor")))

type Leave = "back" | "close" | "saved" | "layout"

export type EditGuard = { leaveThen: (proceed: () => void) => void }

type Props = {
  dashboardId: string
  spec: Dashboard
  panelId: string
  version: number
  latestVersion: number
  pinned: boolean
  view: EditorView
  onViewChange: (view: EditorView) => void
  timeRange: { from?: string; to?: string }
  selections: Selections | null
  onBack: () => void
  onClose: () => void
  onSaved: () => void
  headingId: string
  headingRef: Ref<HTMLHeadingElement>
  guardRef: Ref<EditGuard>
}

function panelTrigger(panelId: string) {
  return document.querySelector<HTMLElement>(
    `[data-panel-menu="${CSS.escape(panelId)}"]`
  )
}

function EditorSkeleton() {
  return (
    <div className="flex flex-col gap-4" aria-busy="true">
      <Skeleton className="h-8 w-full animate-none motion-safe:animate-pulse" />
      <Skeleton className="h-40 w-full animate-none motion-safe:animate-pulse" />
    </div>
  )
}

export function EditPane({
  dashboardId,
  spec,
  panelId,
  version,
  latestVersion,
  pinned,
  view,
  onViewChange,
  timeRange,
  selections,
  onBack,
  onClose,
  onSaved,
  headingId,
  headingRef,
  guardRef,
}: Props) {
  const setSearch = useSetDashboardSearch()
  const navigate = useNavigate()
  useEditorEntryInDocument()
  const returnFocus = useRef(false)
  const afterLeave = useRef<(() => void) | null>(null)
  const onLeave = useCallback(
    (kind: Leave) => {
      if (kind === "layout") {
        const proceed = afterLeave.current
        afterLeave.current = null
        proceed?.()
        return
      }
      if (kind !== "back") returnFocus.current = true
      if (kind === "back") onBack()
      else if (kind === "close") onClose()
      else onSaved()
    },
    [onBack, onClose, onSaved]
  )
  const {
    root,
    dirty,
    onDirtyChange,
    viewGuard,
    focusFirstQuery,
    requestView,
    leave,
    run,
    onKeyDown,
    dialog,
  } = useLeaveGuard<Leave>({
    view,
    onViewChange,
    closeKind: "close",
    leaves: leavesDraft,
    onLeave,
    trigger: () => panelTrigger(panelId),
    discardFocus: (kind) => (kind === "layout" ? "none" : "trigger"),
    keepFocus: () => {
      const focus = afterLeave.current ? "editor" : "default"
      afterLeave.current = null
      return focus
    },
  })
  const holdDraft = useDeletedDashboards((state) => state.holdDraft)
  useEffect(() => {
    holdDraft(dashboardId, dirty)
  }, [dashboardId, dirty, holdDraft])
  useEffect(() => () => holdDraft(dashboardId, false), [dashboardId, holdDraft])
  const deletedBy = useDeletedBy(dashboardId)
  const permissions = useLatestPermissions(dashboardId)
  const provisioned = useLatestProvisioned(dashboardId)
  const patch = usePatchPanels(dashboardId)

  useEffect(
    () => () => {
      if (!returnFocus.current) return
      requestAnimationFrame(() => panelTrigger(panelId)?.focus())
    },
    [panelId]
  )

  const pending = useNewPanels((state) => state.panels[dashboardId])
  const draftTitle = useDraftTitle(panelId)
  const adding =
    pending?.id === panelId && findPanel(spec, panelId) === undefined
  const editorSpec = useMemo(
    () => (adding ? withNewPanel(spec, pending) : spec),
    [adding, spec, pending]
  )
  const current = findPanel(editorSpec, panelId)?.panel
  const [retained, setRetained] = useState<Panel | undefined>()
  if (current && current !== retained) setRetained(current)
  const panel = current ?? (dirty ? retained : undefined)

  const stale = pinned && version !== latestVersion
  const [readOnly, setReadOnly] = useState(stale)
  if (readOnly !== stale && !(stale && dirty)) setReadOnly(stale)

  useImperativeHandle(
    guardRef,
    () => ({
      leaveThen: (proceed) => {
        afterLeave.current = proceed
        leave("layout")
      },
    }),
    [leave]
  )

  const target: DashboardTarget = {
    kind: "dashboard",
    dashboardId,
    latestVersion,
    adding,
    propose: contentChange(permissions, provisioned) === "propose",
    deleted: deletedBy !== undefined,
    save: (input: PanelsSave) =>
      patch.mutateAsync(input).then((outcome) => {
        toast.success(
          saveToast(outcome),
          outcome.kind === "proposed"
            ? {
                action: {
                  label: "View proposal",
                  onClick: () =>
                    void navigate(proposalLink(outcome.proposalId)),
                },
              }
            : undefined
        )
      }),
  }

  const editable = panel !== undefined && isEditablePanel(panel)
  let body: ReactNode
  if (panel === undefined) {
    body = null
  } else if (!isEditablePanel(panel)) {
    body = (
      <div className="flex flex-col items-start gap-3 px-4 py-4">
        <p className="text-[13px] text-muted-foreground">
          This panel cannot be edited.
        </p>
        <Button variant="outline" onClick={() => leave("back")}>
          Back to activity
        </Button>
      </div>
    )
  } else {
    body = (
      <EditorBoundary>
        <Suspense
          fallback={
            <div className="px-4 py-4">
              <EditorSkeleton />
            </div>
          }
        >
          <PanelEditor
            target={target}
            spec={editorSpec}
            panel={panel}
            version={version}
            readOnly={readOnly}
            view={view}
            focusFirstQuery={focusFirstQuery}
            onViewChange={onViewChange}
            viewGuardRef={viewGuard}
            timeRange={timeRange}
            selections={selections}
            onDirtyChange={onDirtyChange}
            onSaved={() => run("saved")}
            onOpenLatest={() =>
              setSearch((state) => ({ ...state, version: undefined }))
            }
            onDiscard={() => leave("back")}
          />
        </Suspense>
      </EditorBoundary>
    )
  }

  return (
    <div
      ref={root}
      className="flex min-h-0 flex-1 flex-col"
      onKeyDown={onKeyDown}
    >
      <DockHeader
        headingId={headingId}
        headingRef={headingRef}
        title={
          adding
            ? "Add a panel"
            : panel
              ? `Edit ${draftTitle ?? panelTitle(panel)}`
              : "Panel not found"
        }
        description={
          adding
            ? "A new panel. Nothing changes until you save."
            : panel
              ? "A draft. Nothing changes until you save."
              : `This panel is not in version ${version}.`
        }
        back={() => leave("back")}
        controls={
          editable ? (
            <EditorViewSwitch view={view} onChange={requestView} />
          ) : null
        }
        onClose={() => leave("close")}
        closeLabel="Close panel"
      />
      {deletedBy === undefined ? null : (
        <Alert variant="destructive" className="mx-4 mt-3 w-auto">
          <AlertDescription>
            {deletedText(deletedBy)} Your draft stays here until you close the
            editor, but it cannot be saved. Copy what you need from the JSON
            view.
          </AlertDescription>
        </Alert>
      )}
      <div className="flex min-h-0 flex-1 flex-col">{body}</div>
      {dialog}
    </div>
  )
}
