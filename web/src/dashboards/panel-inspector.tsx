import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
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
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { CommentsTab } from "@/comments/comments-tab"
import { DashboardComments } from "@/comments/dashboard-comments"
import { useLatestPanels } from "@/comments/latest-panels"
import { useCommentRevision } from "@/revisions/comment-revision"
import type { Dashboard, Panel } from "@/spec"

import {
  useDashboardSearch,
  useSetDashboardSearch,
  validateDashboardSearch,
} from "./search"

const QueryTab = lazy(() => import("@/query-editor/query-tab"))

function panelTrigger(panelId: string | undefined) {
  if (panelId === undefined) return null
  return document.querySelector<HTMLElement>(
    `[data-panel-menu="${CSS.escape(panelId)}"]`
  )
}

function commentsTrigger() {
  return document.querySelector<HTMLElement>("[data-comments-trigger]")
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
    after.tab === undefined ||
    before.version !== after.version
  )
}

function discardCopy(query: boolean, comment: boolean) {
  if (query && comment) {
    return {
      title: "Discard your unsaved changes?",
      description:
        "The query and title you edited and the comment you wrote have not been saved.",
    }
  }
  if (comment) {
    return {
      title: "Discard your comment?",
      description: "The comment you wrote has not been posted.",
    }
  }
  return {
    title: "Discard your query changes?",
    description: "The query and title you edited have not been saved.",
  }
}

type InspectorTab = "query" | "comments"

type InspectorView = { id: InspectorTab; label: string; content: ReactNode }

type Props = {
  spec: Dashboard
  dashboardId: string
  version: number
  latestVersion: number
  pinned: boolean
  suspended: boolean
}

function QuerySkeleton() {
  return (
    <div className="flex flex-col gap-4" aria-busy="true">
      <Skeleton className="h-8 w-full animate-none motion-safe:animate-pulse" />
      <Skeleton className="h-40 w-full animate-none motion-safe:animate-pulse" />
    </div>
  )
}

export function PanelInspector({
  spec,
  dashboardId,
  version,
  latestVersion,
  pinned,
  suspended,
}: Props) {
  const search = useDashboardSearch()
  const setSearch = useSetDashboardSearch()
  const {
    requestRevision,
    composerExtraFields,
    renderRevision,
    onCommentCreated,
  } = useCommentRevision(dashboardId, version, latestVersion)
  const [confirmClose, setConfirmClose] = useState(false)
  const unsavedQuery = useRef(false)
  const [queryDirty, setQueryDirty] = useState(false)
  const [saves, setSaves] = useState(0)
  const onQueryDirty = useCallback((dirty: boolean) => {
    unsavedQuery.current = dirty
    setQueryDirty(dirty)
  }, [])
  const unsavedComment = useRef(false)
  const [commentDirty, setCommentDirty] = useState(false)
  const onCommentDirty = useCallback((dirty: boolean) => {
    unsavedComment.current = dirty
    setCommentDirty(dirty)
  }, [])
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) => {
      return (
        (unsavedQuery.current || unsavedComment.current) &&
        leavesDraft(current, next)
      )
    },
    enableBeforeUnload: () => unsavedQuery.current || unsavedComment.current,
    withResolver: true,
  })
  const discardOpen = confirmClose || blocker.status === "blocked"
  const discarding = useRef(false)

  const panelId = search.panel
  const requested = search.tab !== undefined
  const open = requested && !suspended
  const titleRef = useRef<HTMLHeadingElement>(null)
  const [lastScope, setLastScope] = useState<"panel" | "dashboard">("panel")
  const openScope = panelId === undefined ? "dashboard" : "panel"
  if (open && openScope !== lastScope) setLastScope(openScope)
  const scope = open ? openScope : lastScope

  const previousScope = useRef<string | undefined>(undefined)
  useEffect(() => {
    const now = open ? (panelId ?? "") : undefined
    if (
      previousScope.current !== undefined &&
      now !== undefined &&
      now !== previousScope.current
    ) {
      titleRef.current?.focus()
    }
    previousScope.current = now
  }, [open, panelId])

  useEffect(() => {
    if (suspended && requested) {
      setSearch((state) => ({ ...state, panel: undefined, tab: undefined }), {
        replace: true,
      })
    }
  }, [suspended, requested, setSearch])
  const current =
    open && panelId !== undefined
      ? spec.panels.find((item) => item.id === panelId)
      : undefined
  const [retained, setRetained] = useState<Panel | undefined>()
  if (current && current !== retained) setRetained(current)
  const draftKept = queryDirty || commentDirty
  const kept = draftKept && retained?.id === panelId ? retained : undefined
  const panel = open ? (current ?? kept) : retained

  const latestPanels = useLatestPanels(dashboardId)
  const panelRemoved =
    panel !== undefined &&
    !spec.panels.some((item) => item.id === panel.id) &&
    (version === latestVersion ||
      (latestPanels !== undefined &&
        !latestPanels.some((item) => item.id === panel.id)))

  const stale = pinned && version !== latestVersion
  const [readOnly, setReadOnly] = useState(stale)
  if (readOnly !== stale && !(stale && queryDirty)) setReadOnly(stale)

  const close = useCallback(
    (options?: { dropVersion?: boolean }) => {
      unsavedQuery.current = false
      unsavedComment.current = false
      setConfirmClose(false)
      setSearch((state) => ({
        ...state,
        panel: undefined,
        tab: undefined,
        version: options?.dropVersion ? undefined : state.version,
      }))
    },
    [setSearch]
  )

  const querySaved = useCallback(() => {
    if (unsavedComment.current) {
      unsavedQuery.current = false
      setQueryDirty(false)
      setSaves((count) => count + 1)
      setSearch((state) => ({ ...state, tab: "comments" }), { replace: true })
      return
    }
    close({ dropVersion: true })
  }, [close, setSearch])

  const requestClose = useCallback(() => {
    if (unsavedQuery.current || unsavedComment.current) {
      setConfirmClose(true)
      return
    }
    close()
  }, [close])

  const openLatest = useCallback(() => {
    setSearch((state) => ({ ...state, version: undefined }))
  }, [setSearch])

  const keepEditing = useCallback(() => {
    discarding.current = false
    setConfirmClose(false)
    if (blocker.status === "blocked") blocker.reset()
  }, [blocker])

  const discard = useCallback(() => {
    discarding.current = true
    if (blocker.status === "blocked") {
      unsavedQuery.current = false
      unsavedComment.current = false
      setConfirmClose(false)
      blocker.proceed()
      return
    }
    close()
  }, [blocker, close])

  const views: InspectorView[] = panel
    ? [
        {
          id: "query",
          label: "Query",
          content: (
            <Suspense fallback={<QuerySkeleton />}>
              <QueryTab
                key={`${panel.id}:${saves}`}
                dashboardId={dashboardId}
                spec={spec}
                panel={panel}
                version={version}
                latestVersion={latestVersion}
                readOnly={readOnly}
                onDirtyChange={onQueryDirty}
                onSaved={querySaved}
                onOpenLatest={openLatest}
              />
            </Suspense>
          ),
        },
        {
          id: "comments",
          label: "Comments",
          content: (
            <CommentsTab
              key={panel.id}
              dashboardId={dashboardId}
              panel={panel}
              version={version}
              latestVersion={latestVersion}
              panelRemoved={panelRemoved}
              requestRevision={requestRevision}
              composerExtraFields={composerExtraFields}
              renderRevision={renderRevision}
              onCommentCreated={onCommentCreated}
              onDirtyChange={onCommentDirty}
            />
          ),
        },
      ]
    : []
  const active = views.find((view) => view.id === search.tab) ?? views[0]

  const copy = discardCopy(queryDirty, commentDirty)

  const openPanelComments = useCallback(
    (id: string) =>
      setSearch((state) => ({ ...state, panel: id, tab: "comments" })),
    [setSearch]
  )

  return (
    <>
      <Sheet
        open={open}
        onOpenChange={(next, details) => {
          if (next || discardOpen) return
          if (
            details.reason === "escape-key" &&
            details.event.defaultPrevented
          ) {
            return
          }
          requestClose()
        }}
      >
        <SheetContent
          side="right"
          initialFocus={titleRef}
          className="data-[side=right]:w-[min(720px,100vw)] data-[side=right]:sm:max-w-none"
          finalFocus={() =>
            (scope === "panel"
              ? panelTrigger(retained?.id)
              : commentsTrigger()) ?? true
          }
        >
          {scope === "dashboard" ? (
            <>
              <SheetHeader className="pr-12">
                <SheetTitle
                  ref={titleRef}
                  tabIndex={-1}
                  className="outline-none"
                >
                  Comments
                </SheetTitle>
                <SheetDescription>
                  Comments on every panel of this dashboard.
                </SheetDescription>
              </SheetHeader>
              <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4 pb-4">
                <DashboardComments
                  dashboardId={dashboardId}
                  panels={spec.panels}
                  version={version}
                  latestVersion={latestVersion}
                  onOpenPanel={openPanelComments}
                  renderRevision={renderRevision}
                />
              </div>
            </>
          ) : panel && active ? (
            <>
              <SheetHeader className="pr-12">
                <SheetTitle
                  ref={titleRef}
                  tabIndex={-1}
                  className="wrap-anywhere outline-none"
                >
                  {panel.title}
                </SheetTitle>
                <SheetDescription render={<div />}>
                  <Badge variant="outline">
                    {panel.datasource.type} / {panel.datasource.uid}
                  </Badge>
                </SheetDescription>
              </SheetHeader>
              <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4 pb-4">
                <Tabs
                  value={active.id}
                  className="flex-1"
                  onValueChange={(next) =>
                    setSearch(
                      (state) => ({ ...state, tab: next as InspectorTab }),
                      { replace: true }
                    )
                  }
                >
                  <TabsList variant="line">
                    {views.map((view) => (
                      <TabsTrigger key={view.id} value={view.id}>
                        {view.label}
                      </TabsTrigger>
                    ))}
                  </TabsList>
                  {views.map((view) => (
                    <TabsContent
                      key={view.id}
                      value={view.id}
                      keepMounted
                      className="pt-4 [&:not([hidden])]:flex [&:not([hidden])]:flex-col"
                    >
                      {view.content}
                    </TabsContent>
                  ))}
                </Tabs>
              </div>
            </>
          ) : (
            <>
              <SheetHeader className="pr-12">
                <SheetTitle
                  ref={titleRef}
                  tabIndex={-1}
                  className="outline-none"
                >
                  Panel not found
                </SheetTitle>
                <SheetDescription>
                  This panel is not in version {version}.
                </SheetDescription>
              </SheetHeader>
              <div className="px-4">
                <Button variant="outline" onClick={() => close()}>
                  Close
                </Button>
              </div>
            </>
          )}
          <AlertDialog
            open={discardOpen}
            onOpenChange={(next) => {
              if (!next) keepEditing()
            }}
          >
            <AlertDialogContent
              finalFocus={() =>
                discarding.current
                  ? ((scope === "panel"
                      ? panelTrigger(retained?.id)
                      : commentsTrigger()) ?? true)
                  : true
              }
            >
              <AlertDialogHeader>
                <AlertDialogTitle>{copy.title}</AlertDialogTitle>
                <AlertDialogDescription>
                  {copy.description}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Keep editing</AlertDialogCancel>
                <AlertDialogAction onClick={discard}>Discard</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </SheetContent>
      </Sheet>
    </>
  )
}
