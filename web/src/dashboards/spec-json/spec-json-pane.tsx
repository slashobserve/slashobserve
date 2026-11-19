import { lazy, Suspense, useCallback, useId, useRef, useState } from "react"

import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import { EditorBoundary } from "@/activity/editor-boundary"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
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
import { guardLoad } from "@/panels/renderer-load"
import type { Dashboard } from "@/spec"

const SpecJsonEditor = lazy(guardLoad(() => import("./spec-json-editor")))

type Props = {
  dashboardId: string
  spec: Dashboard
  version: number
  hidden: boolean
  onClose: () => void
}

export function SpecJsonPane({
  dashboardId,
  spec,
  version,
  hidden,
  onClose,
}: Props) {
  const headingId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const layout = useActivityLayout(false)
  const setWidth = useActivityStore((state) => state.setWidth)
  const dirty = useRef(false)
  const [asking, setAsking] = useState(false)
  const onDirtyChange = useCallback((next: boolean) => {
    dirty.current = next
  }, [])

  function requestClose() {
    if (dirty.current) setAsking(true)
    else onClose()
  }

  return (
    <DockPanel
      labelledBy={headingId}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={setWidth}
      stacked={layout.stacked}
      hidden={hidden}
      onKeyDown={(event) => {
        if (event.key !== "Escape" || event.defaultPrevented) return
        event.preventDefault()
        requestClose()
      }}
    >
      <DockHeader
        headingId={headingId}
        headingRef={heading}
        title="Dashboard JSON"
        description={`${spec.title}, version ${version}`}
        onClose={requestClose}
        closeLabel="Close dashboard JSON"
      />
      <EditorBoundary>
        <Suspense
          fallback={
            <div className="flex flex-col gap-3 px-4 py-4">
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-80 w-full" />
            </div>
          }
        >
          <SpecJsonEditor
            dashboardId={dashboardId}
            spec={spec}
            version={version}
            onDirtyChange={onDirtyChange}
            onSaved={onClose}
          />
        </Suspense>
      </EditorBoundary>
      <AlertDialog open={asking} onOpenChange={setAsking}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Discard your JSON edits?</AlertDialogTitle>
            <AlertDialogDescription>
              Your changes to the dashboard JSON are not saved.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep editing</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                setAsking(false)
                onClose()
              }}
            >
              Discard
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DockPanel>
  )
}
