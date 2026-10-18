import { useLayoutEffect, useState, type RefObject } from "react"
import type { Layout } from "react-grid-layout"
import { create } from "zustand"

import type { Dashboard } from "@/spec"

import { layoutFromSpec, sameLayout } from "./layout"

type LayoutSession = {
  dashboardId: string
  baseVersion: number
  baseSpec: Dashboard
  layout: Layout
  movedPanelId: string | null
}

type LayoutState = {
  session: LayoutSession | null
  start: (dashboardId: string, baseVersion: number, baseSpec: Dashboard) => void
  setLayout: (layout: Layout, movedPanelId?: string) => void
  rebase: (baseVersion: number, baseSpec: Dashboard) => void
  end: () => void
}

export const useLayoutStore = create<LayoutState>()((set) => ({
  session: null,
  start: (dashboardId, baseVersion, baseSpec) =>
    set({
      session: {
        dashboardId,
        baseVersion,
        baseSpec,
        layout: layoutFromSpec(baseSpec),
        movedPanelId: null,
      },
    }),
  setLayout: (layout, movedPanelId) =>
    set((state) =>
      state.session
        ? {
            session: {
              ...state.session,
              layout,
              movedPanelId: movedPanelId ?? state.session.movedPanelId,
            },
          }
        : state
    ),
  rebase: (baseVersion, baseSpec) =>
    set((state) => {
      if (!state.session) return state
      const edited = new Map(state.session.layout.map((item) => [item.i, item]))
      const layout = layoutFromSpec(baseSpec).map((item) => {
        const kept = edited.get(item.i)
        return kept
          ? { ...item, x: kept.x, y: kept.y, w: kept.w, h: kept.h }
          : item
      })
      return { session: { ...state.session, baseVersion, baseSpec, layout } }
    }),
  end: () => set({ session: null }),
}))

function sessionIsDirty(session: LayoutSession | null): boolean {
  return (
    session !== null &&
    !sameLayout(session.layout, layoutFromSpec(session.baseSpec))
  )
}

export function useIsDirty(): boolean {
  return useLayoutStore((state) => sessionIsDirty(state.session))
}

export function isDirtyNow(): boolean {
  return sessionIsDirty(useLayoutStore.getState().session)
}

const editMinWidth = 768

export function useGridEditable(ref: RefObject<HTMLElement | null>): boolean {
  const [editable, setEditable] = useState(false)
  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    const measure = () => {
      const style = getComputedStyle(element)
      const width =
        element.clientWidth -
        parseFloat(style.paddingLeft) -
        parseFloat(style.paddingRight)
      setEditable(width >= editMinWidth)
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref])
  return editable
}

export const editLayoutButtonId = "edit-layout-button"

export function focusWhenIdle(find: () => HTMLElement | null) {
  requestAnimationFrame(() => {
    const active = document.activeElement
    if (active && active !== document.body) return
    find()?.focus()
  })
}

export function focusEditLayoutButton() {
  focusWhenIdle(() => document.getElementById(editLayoutButtonId))
}
