import type { ReactNode, Ref } from "react"
import { XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { SidePanelTab } from "@/dashboards/search"

const tabLabels: Record<SidePanelTab, string> = {
  chat: "Chat",
  activity: "Activity",
}

const triggerClass =
  "h-7 flex-none rounded-control px-3 text-[13px] font-medium text-muted-foreground hover:text-foreground data-active:bg-muted data-active:text-foreground dark:data-active:border-transparent dark:data-active:bg-muted"

type Props = {
  headingId: string
  headingRef: Ref<HTMLHeadingElement>
  tab: SidePanelTab
  onTabChange: (tab: SidePanelTab) => void
  onClose: () => void
  children: ReactNode
}

export function SidePanelTabs({
  headingId,
  headingRef,
  tab,
  onTabChange,
  onClose,
  children,
}: Props) {
  return (
    <Tabs
      value={tab}
      onValueChange={(next: SidePanelTab) => onTabChange(next)}
      className="min-h-0 flex-1 gap-0"
    >
      <div
        data-slot="dock-header"
        className="visible flex flex-none items-center justify-between gap-2 border-b py-2.5 pr-3 pl-4"
      >
        <h2
          id={headingId}
          ref={headingRef}
          tabIndex={-1}
          className="sr-only outline-none"
        >
          {tabLabels[tab]}
        </h2>
        <TabsList
          aria-label="Side panel"
          className="h-auto gap-1 bg-transparent p-0"
        >
          <TabsTrigger value="chat" className={triggerClass}>
            {tabLabels.chat}
          </TabsTrigger>
          <TabsTrigger value="activity" className={triggerClass}>
            {tabLabels.activity}
          </TabsTrigger>
        </TabsList>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Close panel"
          onClick={onClose}
        >
          <XIcon aria-hidden="true" className="size-[15px]" />
        </Button>
      </div>
      <TabsContent value={tab} className="flex min-h-0 flex-1 flex-col">
        {children}
      </TabsContent>
    </Tabs>
  )
}
