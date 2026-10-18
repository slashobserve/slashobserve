import { useState } from "react"
import { Outlet } from "@tanstack/react-router"

import { Sidebar } from "./sidebar"
import { PanelHostContext } from "./side-panel"
import { TopRow } from "./top-row"
import { ActionsHostContext } from "./top-row-actions"

export function AppShell() {
  const [panelHost, setPanelHost] = useState<HTMLDivElement | null>(null)
  const [actionsHost, setActionsHost] = useState<HTMLDivElement | null>(null)
  return (
    <PanelHostContext value={panelHost}>
      <ActionsHostContext value={actionsHost}>
        <div className="flex h-svh bg-chrome text-foreground">
          <Sidebar />
          <div className="flex min-w-0 flex-1 flex-wrap gap-2 py-2 pr-2 narrow:pl-2">
            <main className="flex min-h-0 min-w-0 flex-[1_1_560px] flex-col overflow-hidden rounded-card border bg-background">
              <TopRow setActionsHost={setActionsHost} />
              <div className="min-h-0 flex-1 overflow-auto px-6 pt-3.5 pb-7">
                <Outlet />
              </div>
            </main>
            <div ref={setPanelHost} className="contents" />
          </div>
        </div>
      </ActionsHostContext>
    </PanelHostContext>
  )
}
