import {
  useId,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
  type ReactNode,
} from "react"
import { ArrowUpIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { isModEnter, sendsOnEnter } from "@/lib/shortcuts"
import { cleanText } from "@/lib/user-text"
import { sentence } from "@/observe/queries"
import { maxPromptLength } from "@/revisions/queries"

import { SendHint } from "./send-hint"
import { MentionButton } from "./mention-button"
import {
  mentionedAgents,
  type MentionAgent,
  type MentionPerson,
} from "./mentions"
import { useInlineMention } from "./use-inline-mention"
import { useMentionPicks } from "./use-mention-picks"

type ComposerProps = {
  label: string
  formLabel?: string
  placeholder: string
  hint?: ReactNode
  leading?: ReactNode
  people: readonly MentionPerson[]
  target?: string | null
  agents?: readonly MentionAgent[]
  agentsBlocked?: string | null
  onSubmit: (body: string) => Promise<unknown>
  onCancel?: () => void
  autoFocus?: boolean
  disabledReason?: string | null
  defaultValue?: string
}

export const oneAgentText = "Mention one agent per message"

export function agentSendBlock(
  body: string,
  agentsBlocked: string | null | undefined
): string | null {
  const count = mentionedAgents(body).length
  if (count > 1) return oneAgentText
  return count === 1 && typeof agentsBlocked === "string" ? agentsBlocked : null
}

function sendLabel(body: string): string {
  const agents = mentionedAgents(body)
  return agents.length === 1 ? `Send and ask ${agents[0].name}` : "Send"
}

export function SendButton({
  body,
  disabled,
  busy,
}: {
  body: string
  disabled: boolean
  busy: boolean
}) {
  const label = sendLabel(body)
  return (
    <Button
      type="submit"
      size="icon-sm"
      aria-label={label}
      title={label}
      disabled={disabled}
      aria-disabled={busy || undefined}
      aria-busy={busy || undefined}
    >
      <ArrowUpIcon aria-hidden="true" />
    </Button>
  )
}

function describeSendError(error: unknown): string {
  if (error instanceof ApiError && error.status < 500) {
    return sentence(error.message)
  }
  return "The comment could not be sent. Try again."
}

export function Composer({
  label,
  formLabel = label,
  placeholder,
  hint,
  leading,
  people,
  target = null,
  agents = [],
  agentsBlocked,
  onSubmit,
  onCancel,
  autoFocus,
  disabledReason,
  defaultValue = "",
}: ComposerProps) {
  const [body, setBody] = useState(defaultValue)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const mention = useInlineMention()
  const textarea = useRef<HTMLTextAreaElement>(null)
  const coarse = useCoarsePointer()
  const picks = useMentionPicks()
  const text = cleanText(body)
  const wire = picks.wire(text)
  const unavailable = typeof disabledReason === "string"
  const sendBlock = agentSendBlock(wire, agentsBlocked)
  const sendBlockId = useId()

  async function submit() {
    if (busy || unavailable || sendBlock !== null || text === "") return
    setBusy(true)
    setError(null)
    try {
      await onSubmit(wire)
      setBody("")
    } catch (failure) {
      setError(describeSendError(failure))
    } finally {
      setBusy(false)
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLFormElement>) {
    if (event.key === "Escape" && onCancel !== undefined && !busy) {
      event.preventDefault()
      event.stopPropagation()
      onCancel()
    } else if (
      (isModEnter(event) && !event.nativeEvent.isComposing) ||
      (event.target === textarea.current && sendsOnEnter(event, coarse))
    ) {
      event.preventDefault()
      void submit()
    }
  }

  function onChange(event: ChangeEvent<HTMLTextAreaElement>) {
    const { value, selectionStart } = event.target
    mention.typed(body, value, selectionStart)
    setBody(value)
  }

  return (
    <form
      aria-label={formLabel}
      className="flex flex-col rounded-card border border-border bg-card focus-within:ring-2 focus-within:ring-ring"
      onSubmit={(event) => {
        event.preventDefault()
        void submit()
      }}
      onKeyDown={onKeyDown}
    >
      {leading === undefined ? null : (
        <div className="px-1.5 pt-1.5">{leading}</div>
      )}
      <Textarea
        ref={textarea}
        aria-label={label}
        autoFocus={autoFocus}
        rows={2}
        placeholder={placeholder}
        className="field-sizing-content max-h-40 min-h-0 rounded-none border-0 bg-transparent pt-2.5 pr-3 pb-1 pl-3 text-[13px] focus-visible:border-0 focus-visible:ring-0"
        maxLength={maxPromptLength}
        aria-describedby={sendBlock === null ? undefined : sendBlockId}
        value={body}
        disabled={unavailable}
        readOnly={busy}
        onChange={onChange}
      />
      {sendBlock === null ? null : (
        <p id={sendBlockId} className="px-3 text-xs text-muted-foreground">
          {sendBlock}
        </p>
      )}
      {error === null ? null : (
        <p role="alert" className="px-3 text-xs text-destructive">
          {error}
        </p>
      )}
      <div className="flex items-center gap-1 pt-1 pr-1.5 pb-1.5 pl-3">
        <div className="mr-auto min-w-0 text-xs text-muted-foreground">
          {unavailable ? (
            disabledReason
          ) : (
            <>
              {hint}
              <SendHint />
            </>
          )}
        </div>
        <MentionButton
          agents={agents}
          people={people}
          target={target}
          onPick={picks.pick}
          field={textarea}
          value={body}
          onChange={setBody}
          open={mention.open}
          onOpenChange={mention.onOpenChange}
          replaceFrom={mention.replaceFrom}
          disabled={busy || unavailable}
        />
        <SendButton
          body={wire}
          disabled={text === "" || unavailable || sendBlock !== null}
          busy={busy}
        />
      </div>
    </form>
  )
}
