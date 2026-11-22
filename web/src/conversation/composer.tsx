import { useEffect, useId, type KeyboardEvent, type ReactNode } from "react"
import { useRouteContext } from "@tanstack/react-router"
import { ArrowUpIcon } from "lucide-react"
import { toast } from "sonner"

import { agentLabel } from "@/chat/agent-label"
import { ContextChips, type ContextChip } from "@/chat/context-chips"
import { ImportLink } from "@/chat/import-link"
import { NoAgentNote } from "@/chat/no-agent-note"
import { promptCounter } from "@/chat/prompt-length"
import {
  commandHints,
  commandLabels,
  matchingCommands,
} from "@/chat/slash-commands"
import { MentionButton } from "@/comments/mention-button"
import { RunStarted } from "@/comments/run-started"
import { SendHint } from "@/comments/send-hint"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useDatasources } from "@/datasources/queries"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { liveRegionText } from "@/lib/live-region"
import { isEditableTarget, isModEnter, sendsOnEnter } from "@/lib/shortcuts"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
import { StopRequestButton } from "@/observe/request-actions"
import { PromptPicker } from "@/prompts/prompt-picker"
import { usePromptPicker } from "@/prompts/use-prompt-picker"

import { everyoneHere, type ComposerModel } from "./use-composer"

export function ReplyingChip({
  label,
  onLeave,
}: {
  label: string
  onLeave: () => void
}) {
  return (
    <ContextChips
      chips={[
        { id: "replying", label: `Replying in ${label}`, onRemove: onLeave },
      ]}
    />
  )
}

type Layout = "docked" | "page"

type ComposerProps = {
  model: ComposerModel
  layout: Layout
  placeholder: string
  chips?: ReactNode
  label?: string
  formLabel?: string
  leading?: ReactNode
  notice?: ReactNode
  hint?: ReactNode
  disabledReason?: string | null
  rows?: number
  autoFocus?: boolean
  disabled?: boolean
  promptPicker?: boolean
  framed?: boolean
  importLink?: boolean
  contextChips?: ContextChip[]
  showVisibility?: boolean
  fill?: boolean
}

const keyboardHint =
  "Enter sends. Shift+Enter adds a new line. Up arrow recalls an earlier prompt. Type a slash for commands."

const chipClass =
  "inline-flex h-6 max-w-full items-center rounded-chip border px-2 text-xs text-muted-foreground"

function DeliveryChip({
  text,
  turnLabel,
  onTurn,
}: {
  text: string
  turnLabel: string
  onTurn: (() => void) | null
}) {
  if (onTurn === null) {
    return <span className={chipClass}>{text}</span>
  }
  return (
    <Button
      type="button"
      variant="outline"
      size="xs"
      className="max-w-full font-normal"
      aria-label={`${text}. ${turnLabel}`}
      title={turnLabel}
      onClick={onTurn}
    >
      <span className="truncate">{text}</span>
    </Button>
  )
}

function DeliveryChips({
  model,
  showVisibility = true,
}: {
  model: ComposerModel
  showVisibility?: boolean
}) {
  const toAgent = model.delivery.to === "agent"
  const onlyMe = model.delivery.visibleTo === "me"
  return (
    <>
      <DeliveryChip
        text={`To: ${model.recipient}`}
        turnLabel={toAgent ? `Send to ${everyoneHere}` : "Ask the agent"}
        onTurn={model.flips.to ? model.turnTo : null}
      />
      {showVisibility ? (
        <DeliveryChip
          text={`Visible to: ${onlyMe ? "only you" : everyoneHere}`}
          turnLabel={onlyMe ? `Show to ${everyoneHere}` : "Keep it to yourself"}
          onTurn={model.flips.visibleTo ? model.turnVisibleTo : null}
        />
      ) : null}
    </>
  )
}

function sendLabel(model: ComposerModel): string {
  return model.delivery.to === "agent"
    ? `Send and ask ${model.recipient}`
    : "Send"
}

function useKeyDown(
  model: ComposerModel,
  send: () => void,
  extra?: (event: KeyboardEvent<HTMLTextAreaElement>) => boolean
) {
  const coarse = useCoarsePointer()
  return (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (extra?.(event) === true) return
    if (
      (isModEnter(event) && !event.nativeEvent.isComposing) ||
      sendsOnEnter(event, coarse)
    ) {
      event.preventDefault()
      send()
      return
    }
    const element = event.currentTarget
    const plain =
      !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey
    if (
      plain &&
      event.key === "ArrowUp" &&
      (model.text === "" || model.browsingHistory) &&
      element.selectionStart === 0 &&
      element.selectionEnd === 0
    ) {
      if (model.recall("older")) event.preventDefault()
      return
    }
    if (
      plain &&
      event.key === "ArrowDown" &&
      model.browsingHistory &&
      element.selectionStart === element.value.length
    ) {
      if (model.recall("newer")) event.preventDefault()
      return
    }
    if (
      event.key === "Backspace" &&
      model.text === "" &&
      model.command !== null
    ) {
      event.preventDefault()
      model.clearCommand()
    }
  }
}

function Feedback({
  model,
  errorId,
}: {
  model: ComposerModel
  errorId: string
}) {
  return (
    <>
      {model.sendBlock !== null ? (
        <p className="text-xs text-muted-foreground">{model.sendBlock}</p>
      ) : null}
      {model.failure !== null ? (
        <p id={errorId} role="alert" className="text-xs text-status-critical">
          {model.failure.text}
        </p>
      ) : null}
      {model.started === null ? null : (
        <RunStarted
          run={model.started.sent.run}
          refusal={model.started.sent.runRefusal}
          agentName={model.started.agentName}
        />
      )}
      <span role="status" className="sr-only">
        {liveRegionText(model.announcement.text, model.announcement.count)}
      </span>
    </>
  )
}

function SendArrow({
  model,
  label,
  disabled,
  size,
}: {
  model: ComposerModel
  label: string
  disabled: boolean
  size: "icon-sm" | "icon"
}) {
  return (
    <Button
      type="submit"
      size={size}
      aria-label={label}
      title={label}
      disabled={disabled}
      focusableWhenDisabled
      aria-busy={model.pending || undefined}
      className={size === "icon" ? "size-8" : undefined}
    >
      <ArrowUpIcon
        aria-hidden="true"
        className={size === "icon" ? "size-[15px]" : undefined}
      />
    </Button>
  )
}

function DockedComposer({
  model,
  placeholder,
  chips,
  label = "Message",
  formLabel = label,
  leading,
  notice,
  hint,
  disabledReason = null,
  showVisibility,
  fill = false,
}: ComposerProps) {
  const { field } = model
  const errorId = useId()
  const agent = useAgentInfo()
  const modelName = agentLabel(agent.data)
  const onKeyDown = useKeyDown(model, () => void model.submit())
  const unavailable = typeof disabledReason === "string"
  const counter = promptCounter(model.wire)
  return (
    <form
      aria-label={formLabel}
      onSubmit={(event) => {
        event.preventDefault()
        void model.submit()
      }}
      className={cn("flex flex-col gap-2", fill && "min-h-0 flex-auto")}
    >
      {notice}
      <div
        className={cn(
          "rounded-card border border-input bg-background focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/50 has-aria-invalid:border-destructive",
          fill && "flex min-h-0 flex-auto flex-col"
        )}
      >
        <div className="flex flex-wrap items-center gap-1.5 px-2 pt-2">
          {chips}
          <DeliveryChips model={model} showVisibility={showVisibility} />
        </div>
        <Textarea
          ref={field}
          aria-label={label}
          aria-invalid={
            model.failure?.invalid === true ||
            counter?.over === true ||
            undefined
          }
          aria-describedby={model.failure === null ? undefined : errorId}
          placeholder={placeholder}
          value={model.text}
          disabled={unavailable}
          readOnly={model.pending}
          className={cn(
            "field-sizing-content max-h-64 min-h-16 resize-none border-0 px-3 py-2.5 text-[13px] shadow-none focus-visible:ring-0 aria-invalid:ring-0 aria-invalid:focus-visible:ring-0",
            fill && "max-h-none flex-auto"
          )}
          onChange={(event) =>
            model.setText(event.target.value, event.target.selectionStart)
          }
          onKeyDown={onKeyDown}
        />
        <div className="flex items-center gap-2 px-2 pb-2">
          {leading}
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <MentionButton
              agents={model.agents}
              people={model.people}
              target={model.mentionTarget}
              onPick={model.picks.pick}
              field={field}
              value={model.text}
              onChange={model.replaceText}
              open={model.mention.open}
              onOpenChange={model.mention.onOpenChange}
              replaceFrom={model.mention.replaceFrom}
              disabled={model.pending || unavailable}
            />
            {model.runningRequestId !== undefined ? (
              <StopRequestButton requestId={model.runningRequestId} />
            ) : null}
            <SendArrow
              model={model}
              label={sendLabel(model)}
              size="icon-sm"
              disabled={
                model.wire === "" || unavailable || model.sendBlock !== null
              }
            />
          </div>
        </div>
      </div>
      {unavailable ? (
        <p className="text-xs text-muted-foreground">{disabledReason}</p>
      ) : (
        <>
          {hint === undefined ? null : (
            <p className="text-xs text-muted-foreground">{hint}</p>
          )}
          <SendHint />
        </>
      )}
      {model.delivery.to === "agent" && modelName !== null ? (
        <span className="truncate text-xs text-muted-foreground">
          {modelName}
        </span>
      ) : null}
      {counter === null ? null : (
        <span
          className={cn(
            "text-xs text-muted-foreground tabular-nums",
            counter.over && "text-status-critical"
          )}
        >
          {counter.text}
          {counter.over ? ", too long" : null}
        </span>
      )}
      <Feedback model={model} errorId={errorId} />
    </form>
  )
}

function PickerPopup({
  picker,
}: {
  picker: ReturnType<typeof usePromptPicker>
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  return <PromptPicker picker={picker} principalId={principal.id} />
}

function PageComposer({
  model,
  placeholder,
  chips,
  label = "Message the agent",
  rows = 2,
  autoFocus = false,
  disabled = false,
  promptPicker = false,
  framed = true,
  importLink = true,
  contextChips = [],
}: ComposerProps) {
  const { field } = model
  const agent = useAgentInfo()
  const datasources = useDatasources()
  const coarse = useCoarsePointer()
  const inputId = useId()
  const errorId = useId()
  const hintId = useId()
  const counterId = useId()
  const commandHintId = useId()
  const picker = usePromptPicker({
    enabled: promptPicker,
    value: model.text,
    onFill: (body) => {
      model.replaceText(body)
      requestAnimationFrame(() => {
        const element = field.current
        if (element === null) return
        element.setSelectionRange(element.value.length, element.value.length)
      })
    },
  })
  function send() {
    if (agent.isPending && !model.pending) {
      toast.message("Still connecting to the agent. Send again in a moment.")
      return
    }
    if (disabled) return
    void model.submit()
  }
  const onKeyDown = useKeyDown(model, send, (event) =>
    picker.handleKeyDown(event)
  )
  const agentOff = agent.data?.available === false
  const modelName = agentLabel(agent.data)
  const healthy = (datasources.data ?? []).filter((d) => d.healthy)
  const matches = matchingCommands(model.text, model.commands)
  const counter = promptCounter(model.text)

  useEffect(() => {
    if (!autoFocus) return
    const element = field.current
    if (element === null) return
    element.focus()
    element.setSelectionRange(element.value.length, element.value.length)
  }, [autoFocus, agentOff, field])

  useEffect(() => {
    function focusOnSlash(event: globalThis.KeyboardEvent) {
      if (
        event.key !== "/" ||
        event.defaultPrevented ||
        event.isComposing ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey ||
        isEditableTarget(event.target)
      ) {
        return
      }
      const element = field.current
      if (element === null || element.disabled) return
      event.preventDefault()
      element.focus()
    }
    window.addEventListener("keydown", focusOnSlash)
    return () => window.removeEventListener("keydown", focusOnSlash)
  }, [field])

  if (agentOff) return <NoAgentNote />

  const allChips: ContextChip[] = [
    ...(model.command === null
      ? []
      : [
          {
            id: "command",
            label: commandLabels[model.command],
            onRemove: () => {
              model.clearCommand()
              field.current?.focus()
            },
          },
        ]),
    ...contextChips,
  ]
  const describedBy =
    [
      coarse ? null : hintId,
      counter !== null ? counterId : null,
      matches.length > 0 ? commandHintId : null,
      model.failure !== null ? errorId : null,
    ]
      .filter((id) => id !== null)
      .join(" ") || undefined
  const busy = model.pending || agent.isPending

  return (
    <div className="w-full">
      <div className="relative">
        <form
          onSubmit={(event) => {
            event.preventDefault()
            send()
          }}
          className={cn(
            "flex w-full flex-col",
            framed &&
              "rounded-card border bg-background focus-within:ring-2 focus-within:ring-ring has-aria-invalid:border-destructive has-aria-invalid:ring-2 has-aria-invalid:ring-destructive/20 has-aria-invalid:focus-within:border-ring has-aria-invalid:focus-within:ring-ring"
          )}
        >
          <label htmlFor={inputId} className="sr-only">
            {label}
          </label>
          <div className="flex flex-wrap items-center gap-1.5 px-4 pt-3">
            {chips}
            <ContextChips chips={allChips} />
            <DeliveryChips model={model} />
          </div>
          <textarea
            ref={field}
            id={inputId}
            value={model.text}
            rows={rows}
            placeholder={placeholder}
            readOnly={model.pending}
            disabled={disabled}
            aria-invalid={
              model.failure?.invalid === true || counter?.over === true
            }
            aria-describedby={describedBy}
            {...picker.textareaAttributes}
            enterKeyHint={coarse ? "enter" : "send"}
            className="max-h-60 resize-none border-0 bg-transparent px-4 pt-3.5 pb-1.5 text-[15px] leading-[1.5] outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-60 narrow:text-base"
            onChange={(event) => {
              model.setText(event.target.value, event.target.selectionStart)
              picker.onValueChange(event.target.value)
            }}
            onBlur={picker.dismiss}
            onKeyDown={onKeyDown}
          />
          {coarse ? null : (
            <p id={hintId} className="px-4 pb-1 text-xs text-muted-foreground">
              {keyboardHint}
            </p>
          )}
          <div className="flex flex-wrap items-center justify-between gap-2 pt-1.5 pr-2 pb-2 pl-4">
            <div className="flex flex-wrap items-center gap-3.5 text-xs text-muted-foreground">
              {counter === null ? null : (
                <span
                  id={counterId}
                  className={cn(
                    "tabular-nums",
                    counter.over && "font-medium text-destructive"
                  )}
                >
                  {counter.text}
                  {counter.over ? ", too long" : null}
                </span>
              )}
              {healthy.map((datasource) => (
                <span
                  key={datasource.uid}
                  className="flex items-center gap-1.5"
                >
                  <span
                    aria-hidden="true"
                    className="size-[7px] flex-none rounded-full bg-status-ok"
                  />
                  {datasource.uid}
                  <span className="sr-only">, healthy</span>
                </span>
              ))}
              {modelName !== null ? <span>{modelName}</span> : null}
            </div>
            <div className="flex items-center gap-1.5">
              {importLink ? <ImportLink /> : null}
              {model.runningRequestId !== undefined ? (
                <StopRequestButton requestId={model.runningRequestId} />
              ) : null}
              <SendArrow
                model={model}
                label="Send"
                size="icon"
                disabled={busy || disabled}
              />
            </div>
          </div>
        </form>
        {picker.open ? <PickerPopup picker={picker} /> : null}
      </div>
      {picker.announcement !== "" ? (
        <p role="status" className="sr-only">
          {picker.announcement}
        </p>
      ) : null}
      {matches.length > 0 ? (
        <p
          id={commandHintId}
          className="flex flex-wrap gap-x-3 pt-1.5 text-xs text-muted-foreground"
        >
          {matches.map((name) => (
            <span key={name}>
              <span className="font-mono">/{name}</span> {commandHints[name]}
            </span>
          ))}
          <span>Type a space after the command to use it.</span>
        </p>
      ) : null}
      <div className="flex flex-col gap-1 pt-1.5 text-[13px]">
        <Feedback model={model} errorId={errorId} />
      </div>
    </div>
  )
}

export function Composer(props: ComposerProps) {
  return props.layout === "page" ? (
    <PageComposer {...props} />
  ) : (
    <DockedComposer {...props} />
  )
}
