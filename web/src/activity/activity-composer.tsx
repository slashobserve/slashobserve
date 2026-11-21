import {
  useEffect,
  useId,
  useImperativeHandle,
  type ReactNode,
  type Ref,
} from "react"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { MentionPerson } from "@/comments/mentions"
import { Composer, ReplyingChip } from "@/conversation/composer"
import {
  threadPlace,
  type GateInput,
  type ThreadAgent,
} from "@/conversation/delivery"
import { useComposer } from "@/conversation/use-composer"
import { cn } from "@/lib/utils"

import { truncatingTriggerClass } from "./show-filter"

export type Target = { kind: "dashboard" } | { kind: "panel"; panelId: string }

export type Replying = {
  threadId: string
  label: string
  agent: ThreadAgent | null
  agentOff: boolean
  people: readonly MentionPerson[]
}

export const noPanelValue = "*"

export function panelItems(
  noPanelLabel: string,
  panels: readonly { id: string; title: string }[]
): { value: string; label: string }[] {
  return [
    { value: noPanelValue, label: noPanelLabel },
    ...panels.map((panel) => ({
      value: panel.id,
      label: panel.title === "" ? "Untitled panel" : panel.title,
    })),
  ]
}

type Props = {
  dashboardId: string
  panels: readonly { id: string; title: string }[]
  target: Target
  onTargetChange: (target: Target) => void
  replying: Replying | null
  onLeaveThread: () => void
  draft: string
  onDraftChange: (text: string) => void
  gate: GateInput
  placeholder: string
  textareaRef?: Ref<HTMLTextAreaElement>
  notice?: ReactNode
  runningRequestId?: string
}

function TargetSelect({
  panels,
  panelId,
  onTargetChange,
}: {
  panels: readonly { id: string; title: string }[]
  panelId: string | null
  onTargetChange: (target: Target) => void
}) {
  const targetId = useId()
  const items = panelItems("The whole dashboard", panels)
  return (
    <>
      <label
        htmlFor={targetId}
        className="shrink-0 text-xs text-muted-foreground"
      >
        On
      </label>
      <Select
        items={items}
        value={panelId ?? noPanelValue}
        onValueChange={(next) => {
          if (next === null) return
          onTargetChange(
            next === noPanelValue
              ? { kind: "dashboard" }
              : { kind: "panel", panelId: next }
          )
        }}
      >
        <SelectTrigger
          id={targetId}
          size="sm"
          aria-label="On"
          className={cn("max-w-[240px]", truncatingTriggerClass)}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </>
  )
}

export function ActivityComposer({
  dashboardId,
  panels,
  target,
  onTargetChange,
  replying,
  onLeaveThread,
  draft,
  onDraftChange,
  gate,
  placeholder,
  textareaRef,
  notice,
  runningRequestId,
}: Props) {
  const targetMissing =
    target.kind === "panel" &&
    !panels.some((panel) => panel.id === target.panelId)
  const targetGone = targetMissing && panels.length > 0
  useEffect(() => {
    if (targetGone) onTargetChange({ kind: "dashboard" })
  }, [targetGone, onTargetChange])
  const panelId =
    target.kind === "panel" && !targetMissing ? target.panelId : null

  const model = useComposer({
    subject: { kind: "dashboard", id: dashboardId },
    place: threadPlace(
      replying?.threadId ?? null,
      replying === null
        ? null
        : { agent: replying.agent, agent_off: replying.agentOff }
    ),
    target: replying === null && panelId !== null ? { panelId } : undefined,
    gate,
    people: replying?.people,
    draft: { value: draft, onChange: onDraftChange },
    runningRequestId,
    onSent: () => undefined,
  })
  const { field } = model
  useImperativeHandle(textareaRef, () => field.current as HTMLTextAreaElement, [
    field,
  ])

  return (
    <div className="border-t p-3">
      <Composer
        model={model}
        layout="docked"
        label="Comment or request"
        formLabel="Add to the activity"
        placeholder={replying === null ? placeholder : "Reply in this thread"}
        notice={notice}
        chips={
          replying === null ? null : (
            <ReplyingChip
              label={replying.label}
              onLeave={() => {
                onLeaveThread()
                field.current?.focus()
              }}
            />
          )
        }
        leading={
          replying === null ? (
            <TargetSelect
              panels={panels}
              panelId={panelId}
              onTargetChange={onTargetChange}
            />
          ) : null
        }
      />
    </div>
  )
}
