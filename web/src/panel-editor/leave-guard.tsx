import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
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

import type { DraftLocation } from "@/activity/leaves-draft"
import type { EditorView, ViewGuard } from "./panel-editor"

type FocusAfter = "default" | "trigger" | "editor" | "none"

type Options<Kind extends string> = {
  view: EditorView
  onViewChange: (view: EditorView) => void
  closeKind: Kind
  leaves: (current: DraftLocation, next: DraftLocation) => boolean
  onLeave: (kind: Kind) => void
  trigger: () => HTMLElement | null
  discardFocus?: (kind: Kind | null) => FocusAfter
  keepFocus?: () => FocusAfter
}

export type LeaveGuard<Kind extends string> = {
  root: RefObject<HTMLDivElement | null>
  dirty: boolean
  onDirtyChange: (dirty: boolean) => void
  viewGuard: RefObject<ViewGuard | null>
  focusFirstQuery: boolean
  requestView: (view: EditorView) => void
  leave: (kind: Kind) => void
  run: (kind: Kind) => void
  onKeyDown: (event: KeyboardEvent<HTMLDivElement>) => void
  dialog: ReactNode
}

const discardTrigger = () => "trigger" as const
const keepDefault = () => "default" as const

export function useLeaveGuard<Kind extends string>({
  view,
  onViewChange,
  closeKind,
  leaves,
  onLeave,
  trigger,
  discardFocus = discardTrigger,
  keepFocus = keepDefault,
}: Options<Kind>): LeaveGuard<Kind> {
  const unsaved = useRef(false)
  const [dirty, setDirty] = useState(false)
  const onDirtyChange = useCallback((next: boolean) => {
    unsaved.current = next
    setDirty(next)
  }, [])
  const [confirm, setConfirm] = useState<Kind | null>(null)
  const focusAfter = useRef<FocusAfter>("default")
  const root = useRef<HTMLDivElement>(null)
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
      (unsaved.current || jsonOpen.current) && leaves(current, next),
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

  const run = useCallback(
    (kind: Kind) => {
      unsaved.current = false
      jsonOpen.current = false
      setConfirm(null)
      onLeave(kind)
    },
    [onLeave]
  )

  const leave = useCallback(
    (kind: Kind) => {
      const proceed = () => {
        if (unsaved.current) setConfirm(kind)
        else run(kind)
      }
      if (viewGuard.current) viewGuard.current.guardLeave(proceed)
      else proceed()
    },
    [run]
  )

  function keepEditing() {
    focusAfter.current = keepFocus()
    setConfirm(null)
    setBlockConfirmed(false)
    if (blocker.status === "blocked") blocker.reset()
  }

  function discard() {
    focusAfter.current = discardFocus(confirm)
    if (blockConfirmed && blocker.status === "blocked") {
      unsaved.current = false
      jsonOpen.current = false
      setConfirm(null)
      setBlockConfirmed(false)
      blocker.proceed()
      return
    }
    run(confirm ?? closeKind)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "Escape" || event.defaultPrevented || discardOpen) return
    if ((event.target as Element).closest('[role="alertdialog"]')) return
    event.preventDefault()
    // The dialog mounts during this keydown and its own Escape listener would see the same event and close it at once.
    window.setTimeout(() => leave(closeKind), 0)
  }

  function requestView(next: EditorView) {
    setFocusFirstQuery(false)
    if (viewGuard.current) viewGuard.current.request(next)
    else onViewChange(next)
  }

  const dialog = (
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
              return trigger() ?? true
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
  )

  return {
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
  }
}
