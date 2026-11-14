import { useState } from "react"

import { NeedsYouList, PickUpList, WorkingNowList } from "./activity-lists"
import type { HomeTab } from "./home-tab"
import {
  OpenObservationsList,
  RecentChatsList,
  RecentImportsList,
} from "./tab-lists"
import { ExampleChips, TaskCard } from "./task-card"

type Props = {
  tab: HomeTab
  onTabChange: (tab: HomeTab) => void
}

export function HomePage({ tab, onTabChange }: Props) {
  const [autoFocusTab, setAutoFocusTab] = useState<HomeTab | null>(tab)
  const change = (next: HomeTab) => {
    setAutoFocusTab(null)
    onTabChange(next)
  }

  return (
    <div className="mx-auto flex w-full max-w-[720px] flex-col gap-8 pt-[26px] pb-5 narrow:pt-2.5">
      <div className="flex flex-col gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-[-0.01em]">
            What are you working on?
          </h1>
          <p className="mt-1 text-[13px] text-muted-foreground">
            Ask Observe Agent, start an observation, or bring a dashboard over
            from Grafana.
          </p>
        </div>
        <TaskCard tab={tab} onTabChange={change} autoFocusTab={autoFocusTab} />
        {tab === "ask" ? <ExampleChips /> : null}
      </div>
      {tab === "ask" ? <RecentChatsList /> : null}
      {tab === "observe" ? <OpenObservationsList /> : null}
      {tab === "import" ? <RecentImportsList /> : null}
      <NeedsYouList />
      <WorkingNowList />
      <PickUpList />
    </div>
  )
}
