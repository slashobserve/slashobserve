import { create } from "zustand"

import {
  clampSidebarWidth,
  parseStoredWidth,
  sidebarDefaultWidth,
} from "./sidebar-width"

const widthKey = "slashobserve-sidebar-width"
const collapsedKey = "slashobserve-sidebar-collapsed"

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key)
    else localStorage.setItem(key, value)
  } catch {
    return
  }
}

type SidebarState = {
  width: number
  collapsed: boolean
  setWidth: (width: number) => void
  resetWidth: () => void
  setCollapsed: (collapsed: boolean) => void
}

export const useSidebarStore = create<SidebarState>((set, get) => ({
  width: parseStoredWidth(read(widthKey)),
  collapsed: read(collapsedKey) === "true",
  setWidth: (width) => {
    const next = clampSidebarWidth(width)
    write(widthKey, String(next))
    set({ width: next })
  },
  resetWidth: () => get().setWidth(sidebarDefaultWidth),
  setCollapsed: (collapsed) => {
    write(collapsedKey, collapsed ? "true" : null)
    set({ collapsed })
  },
}))
