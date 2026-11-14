import { useId, useRef } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  LoaderCircleIcon,
  NotebookTextIcon,
  TerminalIcon,
  UploadIcon,
} from "lucide-react"

import { ChatComposer, useOpenSent, useSendExample } from "@/chat/chat-composer"
import { examplePrompts } from "@/chat/examples"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useDatasources } from "@/datasources/queries"
import { useAgentInfo } from "@/observe/queries"
import { NewObservationForm } from "@/observations/new-observation-form"
import { useCreateObservation } from "@/observations/queries"
import { dashboardTitlesQuery } from "@/shell/palette-queries"

import { parseHomeTab, type HomeTab } from "./home-tab"
import { ImportStart } from "./import-start"

type Props = {
  tab: HomeTab
  onTabChange: (tab: HomeTab) => void
  autoFocusTab: HomeTab | null
}

const noDashboards: readonly { id: string; title: string }[] = []

const triggerClass =
  "h-7 flex-none gap-1.5 rounded-control px-3 text-[13px] font-medium text-muted-foreground hover:text-foreground data-active:bg-muted data-active:text-foreground dark:data-active:border-transparent dark:data-active:bg-muted narrow:h-11"

function NewObservationTab({ autoFocus }: { autoFocus: boolean }) {
  const formId = `${useId()}-form`
  const titleRef = useRef<HTMLInputElement>(null)
  const create = useCreateObservation()
  return (
    <div className="flex flex-col gap-4 p-4">
      <NewObservationForm
        create={create}
        formId={formId}
        titleRef={titleRef}
        autoFocus={autoFocus}
      />
      <div className="flex justify-end">
        <Button
          type="submit"
          form={formId}
          size="sm"
          focusableWhenDisabled
          disabled={create.isPending}
          aria-busy={create.isPending}
          className="narrow:h-11"
        >
          {create.isPending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="size-3.5 motion-safe:animate-spin"
            />
          ) : null}
          Create
        </Button>
      </div>
    </div>
  )
}

export function TaskCard({ tab, onTabChange, autoFocusTab }: Props) {
  const openSent = useOpenSent()
  return (
    <div className="rounded-card border bg-card focus-within:border-foreground">
      <Tabs
        value={tab}
        onValueChange={(next) => onTabChange(parseHomeTab(next))}
        className="gap-0"
      >
        <TabsList
          aria-label="What to start"
          activateOnFocus
          className="w-full flex-wrap justify-start gap-1 rounded-none rounded-t-card border-b bg-transparent p-2 group-data-horizontal/tabs:h-auto"
        >
          <TabsTrigger value="ask" className={triggerClass}>
            <TerminalIcon aria-hidden="true" />
            Ask Observe Agent
          </TabsTrigger>
          <TabsTrigger value="observe" className={triggerClass}>
            <NotebookTextIcon aria-hidden="true" />
            New observation
          </TabsTrigger>
          <TabsTrigger value="import" className={triggerClass}>
            <UploadIcon aria-hidden="true" />
            Import a dashboard
          </TabsTrigger>
        </TabsList>
        <TabsContent value="ask">
          <ChatComposer
            framed={false}
            importLink={false}
            rows={3}
            placeholder="Describe a dashboard, like: checkout health by pod, last 6h"
            autoFocus={autoFocusTab === "ask"}
            promptPicker
            onSent={openSent}
          />
        </TabsContent>
        <TabsContent value="observe">
          <NewObservationTab autoFocus={autoFocusTab === "observe"} />
        </TabsContent>
        <TabsContent value="import" className="p-4">
          <ImportStart autoFocus={autoFocusTab === "import"} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

export function ExampleChips() {
  const agent = useAgentInfo()
  const datasources = useDatasources()
  const dashboards = useQuery(dashboardTitlesQuery())
  const { send, pending } = useSendExample()

  const examples = examplePrompts({
    types: (datasources.data ?? []).filter((d) => d.healthy).map((d) => d.type),
    dashboards: dashboards.data ?? noDashboards,
  })
  const agentReady = !agent.isPending && agent.data?.available !== false
  if (!agentReady || examples.length === 0) return null

  return (
    <div
      role="group"
      aria-label="Example prompts"
      className="flex flex-wrap gap-1.5"
    >
      {examples.map((example) => (
        <Button
          key={example.label}
          type="button"
          variant="outline"
          size="sm"
          className="font-normal narrow:h-11"
          disabled={pending}
          onClick={() => void send(example.prompt, example.dashboardId)}
        >
          {example.label}
        </Button>
      ))}
    </div>
  )
}
