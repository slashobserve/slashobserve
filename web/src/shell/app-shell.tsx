import { useEffect, useState } from "react"
import { Outlet } from "@tanstack/react-router"

import { InboxPanel } from "@/inbox/inbox-panel"
import { useInboxLive } from "@/inbox/queries"
import { isModChord } from "@/lib/shortcuts"
import { ActivityStreamProvider } from "@/live/activity-stream"

import { CommandPalette } from "./command-palette"
import { HideCrumbContext } from "./crumbs"
import { PhoneBar } from "./phone-bar"
import { Sidebar } from "./sidebar"
import { PanelHostContext } from "./side-panel"
import { sidebarChordKey, sidebarShortcut } from "./sidebar-shortcut"
import { useSidebarStore } from "./sidebar-store"
import { TopRow } from "./top-row"
import { ActionsHostContext } from "./top-row-actions"
import { TitleHostContext } from "./top-row-title"
import { useFoldedByWidth } from "./use-folded"

const phoneQuery = "(max-width: 899px)"

function useSidebarShortcut() {
  const forced = useFoldedByWidth()
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (!isModChord(event, sidebarChordKey)) return
      event.preventDefault()
      const { collapsed, sheetOpen, setCollapsed, setSheetOpen } =
        useSidebarStore.getState()
      const action = sidebarShortcut({
        phone: window.matchMedia(phoneQuery).matches,
        forced,
        collapsed,
        sheetOpen,
      })
      if (action === "collapse") setCollapsed(true)
      else if (action === "expand") setCollapsed(false)
      else if (action === "open-sheet") setSheetOpen(true)
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [forced])
  useEffect(() => {
    if (!forced) useSidebarStore.getState().setSheetOpen(false)
  }, [forced])
}

function InboxLive() {
  useInboxLive()
  return null
}

export function AppShell() {
  useSidebarShortcut()
  const [panelHost, setPanelHost] = useState<HTMLDivElement | null>(null)
  const [actionsHost, setActionsHost] = useState<HTMLDivElement | null>(null)
  const [titleHost, setTitleHost] = useState<HTMLSpanElement | null>(null)
  const [crumbHidden, setCrumbHidden] = useState(false)
  return (
    <ActivityStreamProvider>
      <InboxLive />
      <PanelHostContext value={panelHost}>
        <ActionsHostContext value={actionsHost}>
          <TitleHostContext value={titleHost}>
            <HideCrumbContext value={setCrumbHidden}>
              <div className="flex h-svh bg-chrome text-foreground">
                <Sidebar />
                <div className="group/shell @container flex min-w-0 flex-1 flex-wrap overflow-y-auto bg-background">
                  <main className="flex max-h-full min-h-0 min-w-0 flex-[1_1_560px] flex-col overflow-hidden @max-[1000px]:group-has-[aside]/shell:max-h-none">
                    <PhoneBar />
                    <TopRow
                      setActionsHost={setActionsHost}
                      setTitleHost={setTitleHost}
                      crumbHidden={crumbHidden}
                    />
                    <div className="relative min-h-0 flex-1 overflow-auto px-6 pt-3.5 pb-7 narrow:px-4">
                      <Outlet />
                    </div>
                  </main>
                  <div ref={setPanelHost} className="contents" />
                  <InboxPanel />
                </div>
                <CommandPalette />
              </div>
            </HideCrumbContext>
          </TitleHostContext>
        </ActionsHostContext>
      </PanelHostContext>
    </ActivityStreamProvider>
  )
}
