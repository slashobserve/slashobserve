import { useId, useRef, type CSSProperties, type RefObject } from "react"
import type { QueryKey } from "@tanstack/react-query"

import { useActivityLayout, useActivityStore } from "@/activity/activity-store"
import { CardHeader } from "@/activity/card-header"
import { ResizeHandle } from "@/activity/resize-handle"
import type { Selections } from "@/api/keys"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { PanelDataResponse } from "@/dashboards/queries"
import { useSetDashboardSearch, type InspectTab } from "@/dashboards/search"
import { cn } from "@/lib/utils"
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
    <aside
      id={inspectPaneId}
      aria-labelledby={headingId}
      style={{ "--activity-w": `${layout.width}px` } as CSSProperties}
      className={cn(
        "relative flex min-h-0 min-w-0 flex-col rounded-card border bg-background",
        layout.stacked
          ? "mr-2.5 mb-2.5 ml-4 h-[70svh] flex-[1_1_100%]"
          : "m-2 ml-0.5 flex-[0_0_var(--activity-w)]"
      )}
    >
      {layout.stacked ? null : (
        <ResizeHandle
          width={layout.width}
          bounds={layout.bounds}
          onChange={setWidth}
        />
      )}
      <CardHeader
        headingId={headingId}
        headingRef={heading}
        title="Inspect"
        description={title}
        close={onClose}
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
          <DataTab target={target} title={title} />
        </TabsContent>
        <TabsContent value="query" className="min-h-0 overflow-auto p-4">
          <QueryTab target={target} variables={variables} zone={zone} />
        </TabsContent>
        <TabsContent value="stats" className="min-h-0 overflow-auto p-4">
          <StatsTab target={target} timing={timing} zone={zone} />
        </TabsContent>
      </Tabs>
    </aside>
  )
}
