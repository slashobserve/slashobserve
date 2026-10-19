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
import {
  findPanel,
  isDataPanel,
  panelTitle,
  type Dashboard,
  type Panel,
} from "@/spec"

import { CardHeader } from "./card-header"

const QueryTab = lazy(() => import("@/query-editor/query-tab"))

type Leave = "back" | "close" | "saved" | "layout"

export type EditGuard = { leaveThen: (proceed: () => void) => void }

type Props = {
  dashboardId: string
  spec: Dashboard
  panelId: string
  version: number
  latestVersion: number
  pinned: boolean
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

function QuerySkeleton() {
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
  const focusAfter = useRef<"default" | "trigger" | "none">("default")
  const returnFocus = useRef(false)
  const afterLeave = useRef<(() => void) | null>(null)

  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) =>
      unsaved.current && leavesDraft(current, next),
    enableBeforeUnload: () => unsaved.current,
    withResolver: true,
  })
  const discardOpen = confirm !== null || blocker.status === "blocked"

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
      if (unsaved.current) setConfirm(kind)
      else run(kind)
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
    focusAfter.current = "default"
    afterLeave.current = null
    setConfirm(null)
    if (blocker.status === "blocked") blocker.reset()
  }

  function discard() {
    focusAfter.current = confirm === "layout" ? "none" : "trigger"
    if (blocker.status === "blocked") {
      unsaved.current = false
      setConfirm(null)
      blocker.proceed()
      return
    }
    run(confirm ?? "close")
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "Escape" || event.defaultPrevented || discardOpen) return
    event.preventDefault()
    // The dialog mounts during this keydown and its own Escape listener would see the same event and close it at once.
    window.setTimeout(() => leave("close"), 0)
  }

  let body: ReactNode
  if (panel === undefined) {
    body = null
  } else if (!isDataPanel(panel)) {
    body = (
      <p className="text-[13px] text-muted-foreground">
        This panel has no query to edit.
      </p>
    )
  } else {
    body = (
      <Suspense fallback={<QuerySkeleton />}>
        <QueryTab
          dashboardId={dashboardId}
          spec={spec}
          panel={panel}
          version={version}
          latestVersion={latestVersion}
          readOnly={readOnly}
          onDirtyChange={onDirtyChange}
          onSaved={() => run("saved")}
          onOpenLatest={() =>
            setSearch((state) => ({ ...state, version: undefined }))
          }
        />
      </Suspense>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col" onKeyDown={onKeyDown}>
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
        close={() => leave("close")}
      />
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4 py-4">
        {body}
      </div>
      <AlertDialog
        open={discardOpen}
        onOpenChange={(next) => {
          if (!next) keepEditing()
        }}
      >
        <AlertDialogContent
          finalFocus={() => {
            if (focusAfter.current === "none") return false
            return focusAfter.current === "trigger"
              ? (panelTrigger(panelId) ?? true)
              : true
          }}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>Discard your query changes?</AlertDialogTitle>
            <AlertDialogDescription>
              The query and title you edited have not been saved.
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
