import { useId, useRef, useState } from "react"
import {
  LoaderCircleIcon,
  NotebookTextIcon,
  TerminalIcon,
  UploadIcon,
} from "lucide-react"

import { useOpenSentTurn } from "@/chat/open-sent"
import { composerPlaceholder } from "@/chat/prompt-length"
import { useChooseExample } from "@/chat/use-choose-example"
import { useExamples } from "@/chat/use-examples"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Composer } from "@/conversation/composer"
import { ungated } from "@/conversation/delivery"
import { useComposer } from "@/conversation/use-composer"
import { NewObservationForm } from "@/observations/new-observation-form"
import { useCreateObservation } from "@/observations/queries"

import { parseHomeTab, type HomeTab } from "./home-tab"
import { ImportStart } from "./import-start"

type Props = {
  tab: HomeTab
  onTabChange: (tab: HomeTab) => void
  autoFocusTab: HomeTab | null
}

const triggerClass =
  "h-7 flex-none gap-1.5 rounded-control px-3 text-[13px] font-medium text-muted-foreground hover:text-foreground data-active:bg-muted data-active:text-foreground dark:data-active:border-transparent dark:data-active:bg-muted narrow:h-11"

function NewObservationTab({ autoFocus }: { autoFocus: boolean }) {
  const formId = `${useId()}-form`
  const titleRef = useRef<HTMLInputElement>(null)
  const create = useCreateObservation()
  const [busy, setBusy] = useState(false)
  return (
    <div className="flex flex-col gap-4 p-4">
      <NewObservationForm
        create={create}
        formId={formId}
        titleRef={titleRef}
        autoFocus={autoFocus}
        withNote
        withAgentStart
        onBusyChange={setBusy}
      />
      <div className="flex justify-end">
        <Button
          type="submit"
          form={formId}
          size="sm"
          focusableWhenDisabled
          disabled={busy}
          aria-busy={busy}
          className="narrow:h-11"
        >
          {busy ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="size-3.5 motion-safe:animate-spin"
            />
          ) : null}
          Start observation
        </Button>
      </div>
    </div>
  )
}

function AskTab({ autoFocus }: { autoFocus: boolean }) {
  const model = useComposer({
    subject: null,
    place: { kind: "chat", chatId: null },
    gate: ungated,
    onSent: useOpenSentTurn(),
  })
  return (
    <Composer
      model={model}
      layout="page"
      framed={false}
      importLink={false}
      rows={3}
      placeholder={composerPlaceholder}
      autoFocus={autoFocus}
      promptPicker
    />
  )
}

export function TaskCard({ tab, onTabChange, autoFocusTab }: Props) {
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
          <AskTab autoFocus={autoFocusTab === "ask"} />
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
  const { examples, agentReady } = useExamples()
  const { choose, pending } = useChooseExample()
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
          onClick={() => choose(example)}
        >
          {example.label}
        </Button>
      ))}
    </div>
  )
}
