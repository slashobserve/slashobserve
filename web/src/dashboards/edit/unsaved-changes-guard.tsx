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

import { isDirtyNow, useLayoutStore } from "./layout-store"

type DiscardDialogProps = {
  open: boolean
  onKeep: () => void
  onDiscard: () => void
}

export function DiscardLayoutDialog({
  open,
  onKeep,
  onDiscard,
}: DiscardDialogProps) {
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onKeep()
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Discard unsaved layout changes?</AlertDialogTitle>
          <AlertDialogDescription>
            Your panel moves and resizes have not been saved as a version.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Keep editing</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onDiscard}>
            Discard
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function shouldBlockNavigation({
  current,
  next,
}: {
  current: { pathname: string }
  next: { pathname: string }
}) {
  return isDirtyNow() && current.pathname !== next.pathname
}

function beforeUnload() {
  return isDirtyNow()
}

export function UnsavedChangesGuard() {
  const end = useLayoutStore((state) => state.end)
  const blocker = useBlocker({
    shouldBlockFn: shouldBlockNavigation,
    enableBeforeUnload: beforeUnload,
    withResolver: true,
  })
  return (
    <DiscardLayoutDialog
      open={blocker.status === "blocked"}
      onKeep={() => blocker.reset?.()}
      onDiscard={() => {
        end()
        blocker.proceed?.()
      }}
    />
  )
}
