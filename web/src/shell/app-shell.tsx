import { useState } from "react"
import { Outlet } from "@tanstack/react-router"

import { CommandPalette } from "./command-palette"
import { PhoneBar } from "./phone-bar"
import { Sidebar } from "./sidebar"
import { PanelHostContext } from "./side-panel"
import { TopRow } from "./top-row"
import { ActionsHostContext } from "./top-row-actions"
import { TitleHostContext } from "./top-row-title"

export function AppShell() {
  const [panelHost, setPanelHost] = useState<HTMLDivElement | null>(null)
  const [actionsHost, setActionsHost] = useState<HTMLDivElement | null>(null)
  const [titleHost, setTitleHost] = useState<HTMLSpanElement | null>(null)
  return (
    <PanelHostContext value={panelHost}>
      <ActionsHostContext value={actionsHost}>
        <TitleHostContext value={titleHost}>
          <div className="flex h-svh bg-chrome text-foreground">
            <Sidebar />
            <div className="group/shell @container flex min-w-0 flex-1 flex-wrap overflow-y-auto bg-background">
              <main className="flex max-h-full min-h-0 min-w-0 flex-[1_1_560px] flex-col overflow-hidden @max-[1000px]:group-has-[aside]/shell:max-h-none">
                <PhoneBar />
                <TopRow
                  setActionsHost={setActionsHost}
                  setTitleHost={setTitleHost}
                />
                <div className="relative min-h-0 flex-1 overflow-auto px-6 pt-3.5 pb-7 narrow:px-4">
                  <Outlet />
                </div>
              </main>
              <div ref={setPanelHost} className="contents" />
            </div>
            <CommandPalette />
          </div>
        </TitleHostContext>
      </ActionsHostContext>
    </PanelHostContext>
  )
}
