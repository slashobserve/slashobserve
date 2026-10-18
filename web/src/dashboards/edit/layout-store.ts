import { useLayoutEffect, useState, type RefObject } from "react"
import type { Layout, LayoutItem } from "react-grid-layout"
import { create } from "zustand"

import type { Dashboard } from "@/spec"

import { layoutFromSpec, sameLayout, settle } from "./layout"

type LayoutSession = {
  dashboardId: string
  baseVersion: number
  baseSpec: Dashboard
  baseline: Layout
  layout: Layout
  announcement: string
  announcementCount: number
}

type LayoutState = {
  session: LayoutSession | null
  start: (dashboardId: string, baseVersion: number, baseSpec: Dashboard) => void
  setLayout: (layout: Layout) => void
  commitLayout: (layout: Layout, announcement: string) => void
  rebase: (baseVersion: number, baseSpec: Dashboard) => void
  end: () => void
}

function baselineOf(spec: Dashboard): Layout {
  return settle(layoutFromSpec(spec))
}

function sameBox(a: LayoutItem, b: LayoutItem): boolean {
  return a.x === b.x && a.y === b.y && a.w === b.w && a.h === b.h
}

function mergeEdits(
  oldBaseline: Layout,
  edited: Layout,
  newBase: Layout
): Layout {
  const before = new Map(oldBaseline.map((item) => [item.i, item]))
  const changed = new Map(
    edited
      .filter((item) => {
        const original = before.get(item.i)
        return original !== undefined && !sameBox(original, item)
      })
      .map((item) => [item.i, item])
  )
  return settle(
    newBase.map((item) => {
      const kept = changed.get(item.i)
      return kept
        ? { ...item, x: kept.x, y: kept.y, w: kept.w, h: kept.h }
        : item
    })
  )
}

export const useLayoutStore = create<LayoutState>()((set) => ({
  session: null,
  start: (dashboardId, baseVersion, baseSpec) => {
    const baseline = baselineOf(baseSpec)
    set({
      session: {
        dashboardId,
        baseVersion,
        baseSpec,
        baseline,
        layout: baseline,
        announcement: "",
        announcementCount: 0,
      },
    })
  },
  setLayout: (layout) =>
    set((state) =>
      state.session && !sameLayout(layout, state.session.layout)
        ? { session: { ...state.session, layout, announcement: "" } }
        : state
    ),
  commitLayout: (layout, announcement) =>
    set((state) =>
      state.session
        ? {
            session: {
              ...state.session,
              layout,
              announcement,
              announcementCount: state.session.announcementCount + 1,
            },
          }
        : state
    ),
  rebase: (baseVersion, baseSpec) =>
    set((state) => {
      if (!state.session) return state
      const baseline = baselineOf(baseSpec)
      const layout = mergeEdits(
        state.session.baseline,
        state.session.layout,
        baseline
      )
      return {
        session: {
          ...state.session,
          baseVersion,
          baseSpec,
          baseline,
          layout,
          announcement: "",
        },
      }
    }),
  end: () => set({ session: null }),
}))

function sessionIsDirty(session: LayoutSession | null): boolean {
  return session !== null && !sameLayout(session.layout, session.baseline)
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
