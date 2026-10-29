import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
} from "react"
import { useBlocker } from "@tanstack/react-router"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Skeleton } from "@/components/ui/skeleton"
import {
  useSetDashboardSearch,
  validateDashboardSearch,
} from "@/dashboards/search"
import type { Selections } from "@/api/keys"
import { Button } from "@/components/ui/button"
import { guardLoad } from "@/panels/renderer-load"
import type { EditorView, ViewGuard } from "@/panel-editor/panel-editor"
import { isEditablePanel } from "@/panel-editor/draft"
import { findPanel, panelTitle, type Dashboard, type Panel } from "@/spec"

import { CardHeader } from "./card-header"
import { EditorBoundary } from "./editor-boundary"
import { EditorViewSwitch } from "./editor-view-switch"

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

function leavesDraft(
  current: { pathname: string; search: unknown },
  next: { pathname: string; search: unknown }
): boolean {
  if (current.pathname !== next.pathname) return true
  const before = validateDashboardSearch(
    current.search as Record<string, unknown>
  )
  const after = validateDashboardSearch(next.search as Record<string, unknown>)
  return (
    before.panel !== after.panel ||
    after.edit === undefined ||
    before.version !== after.version
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
  const unsaved = useRef(false)
  const [dirty, setDirty] = useState(false)
  const onDirtyChange = useCallback((next: boolean) => {
    unsaved.current = next
    setDirty(next)
  }, [])
  const [confirm, setConfirm] = useState<Leave | null>(null)
  const focusAfter = useRef<"default" | "trigger" | "editor" | "none">(
    "default"
  )
  const root = useRef<HTMLDivElement>(null)
  const returnFocus = useRef(false)
  const afterLeave = useRef<(() => void) | null>(null)
  const viewGuard = useRef<ViewGuard>(null)
  const jsonOpen = useRef(view === "json")
  const [focusFirstQuery, setFocusFirstQuery] = useState(view === "editor")
  if (focusFirstQuery && view !== "editor") setFocusFirstQuery(false)
  useEffect(() => {
    jsonOpen.current = view === "json"
  }, [view])
  const [blockConfirmed, setBlockConfirmed] = useState(false)

  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) =>
      (unsaved.current || jsonOpen.current) && leavesDraft(current, next),
    enableBeforeUnload: () => unsaved.current,
    withResolver: true,
  })
  const discardOpen = confirm !== null || blockConfirmed

  useEffect(() => {
    if (blocker.status !== "blocked") return
    const { proceed, reset } = blocker
    const approved = () => {
      if (unsaved.current) {
        setBlockConfirmed(true)
        return
      }
      jsonOpen.current = false
      proceed()
    }
    if (viewGuard.current) viewGuard.current.guardLeave(approved, reset)
    else approved()
  }, [blocker])

  useEffect(
    () => () => {
      if (!returnFocus.current) return
      requestAnimationFrame(() => panelTrigger(panelId)?.focus())
    },
    [panelId]
  )

  const current = findPanel(spec, panelId)?.panel
  const [retained, setRetained] = useState<Panel | undefined>()
  if (current && current !== retained) setRetained(current)
  const panel = current ?? (dirty ? retained : undefined)

  const stale = pinned && version !== latestVersion
  const [readOnly, setReadOnly] = useState(stale)
  if (readOnly !== stale && !(stale && dirty)) setReadOnly(stale)

  const run = useCallback(
    (kind: Leave) => {
      unsaved.current = false
      jsonOpen.current = false
      setConfirm(null)
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

  const leave = useCallback(
    (kind: Leave) => {
      const proceed = () => {
        if (unsaved.current) setConfirm(kind)
        else run(kind)
      }
      if (viewGuard.current) viewGuard.current.guardLeave(proceed)
      else proceed()
    },
    [run]
  )

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

  function keepEditing() {
    focusAfter.current = afterLeave.current ? "editor" : "default"
    afterLeave.current = null
    setConfirm(null)
    setBlockConfirmed(false)
    if (blocker.status === "blocked") blocker.reset()
  }

  function discard() {
    focusAfter.current = confirm === "layout" ? "none" : "trigger"
    if (blockConfirmed && blocker.status === "blocked") {
      unsaved.current = false
      jsonOpen.current = false
      setConfirm(null)
      setBlockConfirmed(false)
      blocker.proceed()
      return
    }
    run(confirm ?? "close")
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "Escape" || event.defaultPrevented || discardOpen) return
    if ((event.target as Element).closest('[role="alertdialog"]')) return
    event.preventDefault()
    // The dialog mounts during this keydown and its own Escape listener would see the same event and close it at once.
    window.setTimeout(() => leave("close"), 0)
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
            dashboardId={dashboardId}
            spec={spec}
            panel={panel}
            version={version}
            latestVersion={latestVersion}
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
      <CardHeader
        headingId={headingId}
        headingRef={headingRef}
        title={panel ? `Edit ${panelTitle(panel)}` : "Panel not found"}
        description={
          panel
            ? "A draft. Nothing changes until you save."
            : `This panel is not in version ${version}.`
        }
        back={() => leave("back")}
        controls={
          editable ? (
            <EditorViewSwitch
              view={view}
              onChange={(next) => {
                setFocusFirstQuery(false)
                if (viewGuard.current) viewGuard.current.request(next)
                else onViewChange(next)
              }}
            />
          ) : null
        }
        close={() => leave("close")}
      />
      <div className="flex min-h-0 flex-1 flex-col">{body}</div>
      <AlertDialog
        open={discardOpen}
        onOpenChange={(next) => {
          if (!next) keepEditing()
        }}
      >
        <AlertDialogContent
          finalFocus={() => {
            switch (focusAfter.current) {
              case "none":
                return false
              case "trigger":
                return panelTrigger(panelId) ?? true
              case "editor":
                return (
                  root.current?.querySelector<HTMLElement>(".cm-content") ??
                  root.current?.querySelector<HTMLElement>("h2") ??
                  true
                )
              default:
                return true
            }
          }}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>Discard your changes?</AlertDialogTitle>
            <AlertDialogDescription>
              The panel changes you made have not been saved.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep editing</AlertDialogCancel>
            <AlertDialogAction onClick={discard}>Discard</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
