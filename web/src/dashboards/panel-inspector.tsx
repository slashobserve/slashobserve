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

function leavesQueryEditor(
  current: { pathname: string; search: unknown },
  next: { pathname: string; search: unknown }
) {
  if (current.pathname !== next.pathname) return true
  const before = validateDashboardSearch(
    current.search as Record<string, unknown>
  )
  const after = validateDashboardSearch(next.search as Record<string, unknown>)
  return (
    before.panel !== after.panel ||
    before.version !== after.version ||
    after.tab === undefined
  )
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
  const [confirmClose, setConfirmClose] = useState(false)
  const unsavedQuery = useRef(false)
  const [queryDirty, setQueryDirty] = useState(false)
  const onQueryDirty = useCallback((dirty: boolean) => {
    unsavedQuery.current = dirty
    setQueryDirty(dirty)
  }, [])
  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) =>
      unsavedQuery.current && leavesQueryEditor(current, next),
    enableBeforeUnload: () => unsavedQuery.current,
    withResolver: true,
  })
  const discardOpen = confirmClose || blocker.status === "blocked"
  const discarding = useRef(false)

  const panelId = search.panel
  const requested = panelId !== undefined && search.tab !== undefined
  const open = requested && !suspended
  const titleRef = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    if (suspended && requested) {
      setSearch((state) => ({ ...state, panel: undefined, tab: undefined }), {
        replace: true,
      })
    }
  }, [suspended, requested, setSearch])
  const current = open
    ? spec.panels.find((item) => item.id === panelId)
    : undefined
  const [retained, setRetained] = useState<Panel | undefined>()
  if (current && current !== retained) setRetained(current)
  const kept = queryDirty && retained?.id === panelId ? retained : undefined
  const panel = open ? (current ?? kept) : retained

  const stale = pinned && version !== latestVersion
  const [readOnly, setReadOnly] = useState(stale)
  if (readOnly !== stale && !(stale && queryDirty)) setReadOnly(stale)

  const close = useCallback(
    (options?: { dropVersion?: boolean }) => {
      unsavedQuery.current = false
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

  const requestClose = useCallback(() => {
    if (unsavedQuery.current) {
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
                key={panel.id}
                dashboardId={dashboardId}
                spec={spec}
                panel={panel}
                version={version}
                latestVersion={latestVersion}
                readOnly={readOnly}
                onDirtyChange={onQueryDirty}
                onSaved={() => close({ dropVersion: true })}
                onOpenLatest={openLatest}
              />
            </Suspense>
          ),
        },
      ]
    : []
  const active = views.find((view) => view.id === search.tab) ?? views[0]

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
          finalFocus={() => panelTrigger(retained?.id) ?? true}
        >
          {panel && active ? (
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
                      className="pt-4"
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
                discarding.current ? (panelTrigger(retained?.id) ?? true) : true
              }
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
        </SheetContent>
      </Sheet>
    </>
  )
}
