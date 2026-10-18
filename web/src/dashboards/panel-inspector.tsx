import {
  lazy,
  Suspense,
  useCallback,
  useRef,
  useState,
  type ReactNode,
} from "react"

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

import { useDashboardSearch, useSetDashboardSearch } from "./search"

const QueryTab = lazy(() => import("@/query-editor/query-tab"))

type InspectorTab = "query" | "comments"

type InspectorView = { id: InspectorTab; label: string; content: ReactNode }

type Props = {
  spec: Dashboard
  dashboardId: string
  version: number
  latestVersion: number
  pinned: boolean
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
}: Props) {
  const search = useDashboardSearch()
  const setSearch = useSetDashboardSearch()
  const [confirmClose, setConfirmClose] = useState(false)
  const unsavedQuery = useRef(false)
  const onQueryDirty = useCallback((dirty: boolean) => {
    unsavedQuery.current = dirty
  }, [])

  const panelId = search.panel
  const open = panelId !== undefined && search.tab !== undefined
  const current = open
    ? spec.panels.find((item) => item.id === panelId)
    : undefined
  const [retained, setRetained] = useState<Panel | undefined>()
  if (current && current !== retained) setRetained(current)
  const panel = open ? current : retained

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
    unsavedQuery.current = false
    setSearch((state) => ({ ...state, version: undefined }))
  }, [setSearch])

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
                readOnly={pinned && version !== latestVersion}
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
          if (next) return
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
          className="data-[side=right]:w-[min(720px,100vw)] data-[side=right]:sm:max-w-none"
          finalFocus={() =>
            retained === undefined
              ? true
              : (document.querySelector<HTMLElement>(
                  `[data-panel-menu="${CSS.escape(retained.id)}"]`
                ) ?? true)
          }
        >
          {panel && active ? (
            <>
              <SheetHeader className="pr-12">
                <SheetTitle className="wrap-anywhere">{panel.title}</SheetTitle>
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
                <SheetTitle>Panel not found</SheetTitle>
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
        </SheetContent>
      </Sheet>
      <AlertDialog open={confirmClose} onOpenChange={setConfirmClose}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard your query changes?</AlertDialogTitle>
            <AlertDialogDescription>
              The query and title you edited have not been saved.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep editing</AlertDialogCancel>
            <AlertDialogAction onClick={() => close()}>
              Discard
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
