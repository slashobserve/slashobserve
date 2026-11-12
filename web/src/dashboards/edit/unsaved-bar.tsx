import { useState } from "react"

import { Button } from "@/components/ui/button"

import {
  findMoreMenuButton,
  findMoveHandle,
  focusAfterLayoutChange,
  isDirtyNow,
  useIsDirty,
  useLayoutStore,
} from "./layout-store"
import { SaveLayoutDialog } from "./save-layout-dialog"
import { DiscardLayoutDialog } from "./unsaved-changes-guard"

export function UnsavedBar({ dashboardId }: { dashboardId: string }) {
  const dirty = useIsDirty()
  const nextVersion = useLayoutStore((state) =>
    state.session?.dashboardId === dashboardId
      ? state.session.baseVersion + 1
      : null
  )
  const end = useLayoutStore((state) => state.end)
  const [saveOpen, setSaveOpen] = useState(false)
  const [discardOpen, setDiscardOpen] = useState(false)
  if (!dirty || nextVersion === null) return null
  return (
    <div
      role="region"
      aria-label="Unsaved layout"
      className="fixed bottom-16 left-1/2 z-30 flex max-w-[calc(100vw-2rem)] -translate-x-1/2 flex-wrap items-center justify-center gap-x-3 gap-y-2 rounded-control border border-border bg-popover px-3 py-2 text-popover-foreground narrow:bottom-4"
    >
      <p className="text-[13px]">Your layout changes are not saved</p>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => setDiscardOpen(true)}
        >
          Discard
        </Button>
        <Button size="sm" onClick={() => setSaveOpen(true)}>
          Save version {nextVersion}
        </Button>
      </div>
      <SaveLayoutDialog
        dashboardId={dashboardId}
        open={saveOpen}
        onOpenChange={setSaveOpen}
      />
      <DiscardLayoutDialog
        open={discardOpen}
        finalFocus={() =>
          isDirtyNow() ? (findMoveHandle() ?? findMoreMenuButton()) : false
        }
        onKeep={() => setDiscardOpen(false)}
        onDiscard={() => {
          setDiscardOpen(false)
          end()
          focusAfterLayoutChange()
        }}
      />
    </div>
  )
}
