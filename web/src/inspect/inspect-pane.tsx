import { useId, useRef, type RefObject } from "react"
import type { QueryKey } from "@tanstack/react-query"

import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import type { Selections } from "@/api/keys"
import { DockHeader } from "@/components/dock/dock-header"
import { DockPanel } from "@/components/dock/dock-panel"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { PanelDataResponse } from "@/dashboards/queries"
import { useSetDashboardSearch, type InspectTab } from "@/dashboards/search"
import type { PreparedFrame } from "@/panels/transforms/names"
import type { DataPanel, Variable } from "@/spec"

import { DataTab } from "./data-tab"
import { QueryTab } from "./query-tab"
import { useRequestTiming } from "./request-timing"
import { StatsTab } from "./stats-tab"

export type InspectTarget = {
  panel: DataPanel
  title: string
  data: PanelDataResponse | undefined
  frames: PreparedFrame[] | undefined
  queryKey: QueryKey
  selections: Selections
  body: RefObject<HTMLElement | null>
  refetch: () => void
}

export const inspectPaneId = "inspect-pane"

const triggerClass =
  "h-auto flex-none rounded-none border-0 border-b-2 border-transparent px-0 pb-2 text-[13px] font-medium text-muted-foreground -mb-px after:hidden data-active:border-foreground data-active:text-foreground dark:data-active:border-foreground dark:group-data-[variant=line]/tabs-list:data-active:border-foreground"

type Props = {
  title: string
  tab: InspectTab
  target: InspectTarget | null
  variables: readonly Variable[]
  zone: string | undefined
  onClose: () => void
}

export function InspectPane({
  title,
  tab,
  target,
  variables,
  zone,
  onClose,
}: Props) {
  const headingId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const layout = useActivityLayout(false)
  const setWidth = useActivityStore((state) => state.setWidth)
  const setSearch = useSetDashboardSearch()
  const timing = useRequestTiming(target?.queryKey)
  return (
    <DockPanel
      id={inspectPaneId}
      labelledBy={headingId}
      width={layout.width}
      bounds={layout.bounds}
      onWidthChange={setWidth}
      stacked={layout.stacked}
    >
      <DockHeader
        headingId={headingId}
        headingRef={heading}
        title="Inspect"
        description={title}
        onClose={onClose}
        closeLabel="Close panel"
      />
      <Tabs
        value={tab}
        onValueChange={(value: InspectTab) =>
          setSearch((state) => ({ ...state, inspectTab: value }), {
            replace: true,
          })
        }
        className="min-h-0 flex-1 gap-0"
      >
        <TabsList
          variant="line"
          activateOnFocus
          aria-label="Inspect"
          className="h-auto w-full flex-none justify-start gap-[18px] border-b px-4 pt-3"
        >
          <TabsTrigger value="data" className={triggerClass}>
            Data
          </TabsTrigger>
          <TabsTrigger value="query" className={triggerClass}>
            Query
          </TabsTrigger>
          <TabsTrigger value="stats" className={triggerClass}>
            Stats
          </TabsTrigger>
        </TabsList>
        <TabsContent value="data" className="min-h-0 overflow-auto p-4">
          <DataTab target={target} title={title} zone={zone} />
        </TabsContent>
        <TabsContent value="query" className="min-h-0 overflow-auto p-4">
          <QueryTab target={target} variables={variables} zone={zone} />
        </TabsContent>
        <TabsContent value="stats" className="min-h-0 overflow-auto p-4">
          <StatsTab target={target} timing={timing} zone={zone} />
        </TabsContent>
      </Tabs>
    </DockPanel>
  )
}
