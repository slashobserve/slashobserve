import { useSyncExternalStore } from "react"
import type { QueryClient } from "@tanstack/react-query"
import { create } from "zustand"
import { persist } from "zustand/middleware"

import { activityWidth, clampWidth, editWidth, isStacked } from "./card-width"

const storageKey = "slashobserve-activity"
const retiredReviseKey = "slashobserve-revise-on-comment"

try {
  localStorage.removeItem(retiredReviseKey)
} catch {
  // Storage can be unavailable (private mode); the retired preference then has nothing to remove.
}

type Persisted = { open: boolean; width: number; editWidth: number }

type ActivityState = Persisted & {
  drafts: Record<string, string>
  targets: Record<string, string | null>
  setOpen: (open: boolean) => void
  setWidth: (width: number) => void
  setEditWidth: (width: number) => void
  setDraft: (dashboardId: string, text: string) => void
  setTarget: (dashboardId: string, panelId: string | null) => void
}

function sanitize(saved: unknown, current: ActivityState): ActivityState {
  if (typeof saved !== "object" || saved === null) return current
  const stored = saved as Partial<Record<keyof Persisted, unknown>>
  const number = (value: unknown, fallback: number) =>
    typeof value === "number" ? value : fallback
  return {
    ...current,
    open: typeof stored.open === "boolean" ? stored.open : current.open,
    width: clampWidth(
      number(stored.width, current.width),
      activityWidth,
      Number.POSITIVE_INFINITY
    ),
    editWidth: clampWidth(
      number(stored.editWidth, current.editWidth),
      editWidth,
      Number.POSITIVE_INFINITY
    ),
  }
}

export const useActivityStore = create<ActivityState>()(
  persist(
    (set) => ({
      open: true,
      width: activityWidth.initial,
      editWidth: editWidth.initial,
      drafts: {},
      targets: {},
      setOpen: (open) => set({ open }),
      setWidth: (width) =>
        set({
          width: clampWidth(width, activityWidth, Number.POSITIVE_INFINITY),
        }),
      setEditWidth: (width) =>
        set({
          editWidth: clampWidth(width, editWidth, Number.POSITIVE_INFINITY),
        }),
      setDraft: (dashboardId, text) =>
        set((state) => ({ drafts: { ...state.drafts, [dashboardId]: text } })),
      setTarget: (dashboardId, panelId) =>
        set((state) => ({
          targets: { ...state.targets, [dashboardId]: panelId },
        })),
    }),
    {
      name: storageKey,
      partialize: ({ open, width, editWidth }) => ({ open, width, editWidth }),
      merge: sanitize,
    }
  )
)

export function useDraft(dashboardId: string): string {
  return useActivityStore((state) =>
    Object.hasOwn(state.drafts, dashboardId) ? state.drafts[dashboardId] : ""
  )
}

export function useStoredTarget(dashboardId: string): string | null {
  return useActivityStore((state) =>
    Object.hasOwn(state.targets, dashboardId)
      ? state.targets[dashboardId]
      : null
  )
}

function postedText(variables: unknown): string | undefined {
  if (typeof variables !== "object" || variables === null) return undefined
  if ("parentId" in variables || "commentId" in variables) return undefined
  if ("body" in variables && typeof variables.body === "string") {
    return variables.body
  }
  if ("prompt" in variables && typeof variables.prompt === "string") {
    return variables.prompt
  }
  return undefined
}

function postedDashboardId(key: readonly unknown[] | undefined) {
  if (key?.length !== 3 || key[0] !== "dashboards" || key[2] !== "posts") {
    return undefined
  }
  return typeof key[1] === "string" ? key[1] : undefined
}

// The composer unmounts when the card closes or the dashboard is left, and a mutation callback passed to mutate() never runs after that.
// This listens to the mutation cache for the whole app, so a comment or request that finishes later still empties its draft.
export function watchPostedDrafts(queryClient: QueryClient) {
  return queryClient.getMutationCache().subscribe((event) => {
    if (event.type !== "updated" || event.action.type !== "success") return
    const dashboardId = postedDashboardId(event.mutation.options.mutationKey)
    const text = postedText(event.mutation.state.variables)
    if (dashboardId === undefined || text === undefined) return
    const { drafts, setDraft } = useActivityStore.getState()
    if (
      Object.hasOwn(drafts, dashboardId) &&
      drafts[dashboardId].trim() === text
    ) {
      setDraft(dashboardId, "")
    }
  })
}

function subscribeToResize(callback: () => void) {
  window.addEventListener("resize", callback)
  return () => window.removeEventListener("resize", callback)
}

function useViewportWidth(): number {
  return useSyncExternalStore(
    subscribeToResize,
    () => window.innerWidth,
    () => 1280
  )
}

export function useActivityLayout(editing: boolean) {
  const stored = useActivityStore((state) =>
    editing ? state.editWidth : state.width
  )
  const viewport = useViewportWidth()
  const bounds = editing ? editWidth : activityWidth
  return {
    bounds,
    viewport,
    width: clampWidth(stored, bounds, viewport),
    stacked: isStacked(bounds, viewport),
  }
}
