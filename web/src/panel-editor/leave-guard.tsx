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
import { createStore, useStore } from "zustand"

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
import {
  asksToDiscard,
  hasUnsavedEdits,
  noUnsavedEdits,
  reduceLeave,
  type LeaveEvent,
  type LeaveState,
} from "./leave-state"
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
  const [store] = useState(() =>
    createStore<LeaveState<Kind>>(() => noUnsavedEdits)
  )
  const state = useStore(store)
  const dispatch = useCallback(
    (event: LeaveEvent<Kind>) =>
      store.setState(reduceLeave(store.getState(), event), true),
    [store]
  )
  const unsavedNow = useCallback(
    () => hasUnsavedEdits(store.getState()),
    [store]
  )
  const dirty = hasUnsavedEdits(state)
  const onDirtyChange = useCallback(
    (next: boolean) => dispatch({ type: "edited", dirty: next }),
    [dispatch]
  )
  const focusAfter = useRef<FocusAfter>("default")
  const root = useRef<HTMLDivElement>(null)
  const viewGuard = useRef<ViewGuard>(null)
  const jsonOpen = useRef(view === "json")
  const [focusFirstQuery, setFocusFirstQuery] = useState(view === "editor")
  if (focusFirstQuery && view !== "editor") setFocusFirstQuery(false)
  useEffect(() => {
    jsonOpen.current = view === "json"
  }, [view])

  const blocker = useBlocker({
    shouldBlockFn: ({ current, next }) =>
      (unsavedNow() || jsonOpen.current) && leaves(current, next),
    enableBeforeUnload: unsavedNow,
    withResolver: true,
  })
  const discardOpen = asksToDiscard(state)

  useEffect(() => {
    if (blocker.status !== "blocked") return
    const { proceed, reset } = blocker
    const approved = () => {
      if (unsavedNow()) {
        dispatch({ type: "navigation_held" })
        return
      }
      jsonOpen.current = false
      proceed()
    }
    if (viewGuard.current) viewGuard.current.guardLeave(approved, reset)
    else approved()
  }, [blocker, dispatch, unsavedNow])

  const run = useCallback(
    (kind: Kind) => {
      jsonOpen.current = false
      dispatch({ type: "left" })
      onLeave(kind)
    },
    [dispatch, onLeave]
  )

  const leave = useCallback(
    (kind: Kind) => {
      const proceed = () => {
        if (unsavedNow()) dispatch({ type: "asked", kind })
        else run(kind)
      }
      if (viewGuard.current) viewGuard.current.guardLeave(proceed)
      else proceed()
    },
    [dispatch, run, unsavedNow]
  )

  function keepEditing() {
    focusAfter.current = keepFocus()
    dispatch({ type: "kept" })
    if (blocker.status === "blocked") blocker.reset()
  }

  function discard() {
    const asked = state.status === "confirming" ? state.kind : null
    focusAfter.current = discardFocus(asked)
    if (state.status === "blocked" && blocker.status === "blocked") {
      jsonOpen.current = false
      dispatch({ type: "left" })
      blocker.proceed()
      return
    }
    run(asked ?? closeKind)
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
