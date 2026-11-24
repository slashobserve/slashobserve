import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react"
import { AtSignIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
} from "@/components/ui/combobox"
import { cn } from "@/lib/utils"

import {
  inlineKeyStep,
  inlineMentionStep,
  insertMention,
  matchingOptions,
  mentionOptions,
  peopleOptions,
  typedMentionEnd,
  type MentionAgent,
  type MentionOption,
  type MentionPerson,
} from "./mentions"
import { useMentionablePeople } from "./use-mentionable-people"

const searchDelayMs = 200

type MentionButtonProps = {
  agents: readonly MentionAgent[]
  people: readonly MentionPerson[]
  target: string | null
  onPick: (option: MentionOption) => string
  field: RefObject<HTMLTextAreaElement | null>
  value: string
  onChange: (next: string) => void
  open: boolean
  onOpenChange: (open: boolean) => void
  replaceFrom: number | null
  disabled?: boolean
  className?: string
}

function offered(
  agents: readonly MentionAgent[],
  people: readonly MentionPerson[],
  found: readonly MentionPerson[],
  query: string
): MentionOption[] {
  const local = matchingOptions(mentionOptions(people, agents), query)
  const seen = new Set(local.map((option) => option.value))
  return [
    ...local,
    ...peopleOptions(found).filter((option) => !seen.has(option.value)),
  ]
}

export function MentionButton({
  agents,
  people,
  target,
  onPick,
  field,
  value,
  onChange,
  open,
  onOpenChange,
  replaceFrom,
  disabled,
  className,
}: MentionButtonProps) {
  const [query, setQuery] = useState("")
  const [term, setTerm] = useState("")
  const [moved, setMoved] = useState<{ query: string; index: number } | null>(
    null
  )
  const inline = open && replaceFrom !== null
  const caret = field.current?.selectionStart ?? value.length
  const typed =
    inline && value[replaceFrom] === "@"
      ? value.slice(replaceFrom + 1, Math.max(caret, replaceFrom + 1))
      : ""
  const search = inline ? typed : query
  useEffect(() => {
    const timer = setTimeout(() => setTerm(search.trim()), searchDelayMs)
    return () => clearTimeout(timer)
  }, [search])
  const found = useMentionablePeople(target, term, open)
  const options = offered(agents, people, found.people, search)
  const searching = found.searching || term !== search.trim()
  const step = inline
    ? inlineMentionStep({
        value,
        trigger: replaceFrom,
        caret,
        matching: options.length,
        searching: target !== null && searching,
      })
    : null
  const active =
    moved !== null && moved.query === typed
      ? moved.index
      : typed === "" || options.length === 0
        ? -1
        : 0

  function close() {
    setQuery("")
    setMoved(null)
    onOpenChange(false)
  }

  function insert(option: MentionOption) {
    const element = field.current
    const start = replaceFrom ?? element?.selectionStart ?? value.length
    const end =
      replaceFrom === null
        ? (element?.selectionEnd ?? value.length)
        : typedMentionEnd(value, replaceFrom, element?.selectionStart ?? 0)
    const next = insertMention(value, start, end, onPick(option))
    onChange(next.value)
    requestAnimationFrame(() => {
      const target = field.current
      if (target === null) return
      target.focus()
      target.setSelectionRange(next.caret, next.caret)
    })
  }

  const stale = step !== null && !step.open
  const keys = useRef({ options, active, insert, close, typed, searching })
  useEffect(() => {
    keys.current = { options, active, insert, close, typed, searching }
  })
  useEffect(() => {
    if (stale) keys.current.close()
  }, [stale])
  useEffect(() => {
    const element = field.current
    if (!inline || element === null) return
    function onFieldKey(event: globalThis.KeyboardEvent) {
      if (event.isComposing) return
      const now = keys.current
      const result = inlineKeyStep({
        key: event.key,
        active: now.active,
        count: now.options.length,
        modified:
          event.shiftKey || event.metaKey || event.ctrlKey || event.altKey,
        searching: now.searching,
      })
      if (result.kind === "pass") return
      event.preventDefault()
      event.stopPropagation()
      if (result.kind === "hold") return
      if (result.kind === "close") now.close()
      else if (result.kind === "move")
        setMoved({ query: now.typed, index: result.index })
      else {
        now.insert(now.options[result.index])
        now.close()
      }
    }
    element.addEventListener("keydown", onFieldKey)
    return () => element.removeEventListener("keydown", onFieldKey)
  }, [inline, field])

  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === "Escape" && open) event.stopPropagation()
  }

  return (
    <Combobox<MentionOption>
      items={options}
      filter={null}
      value={null}
      open={open}
      highlightItemOnHover={!inline}
      inputValue={search}
      disabled={disabled}
      itemToStringLabel={(option) => option.label}
      onOpenChange={(next) => {
        if (!next) setQuery("")
        onOpenChange(next)
      }}
      onInputValueChange={(text, details) =>
        setQuery(details.reason === "item-press" ? "" : text)
      }
      onValueChange={(next) => {
        if (next !== null) insert(next)
      }}
    >
      <ComboboxTrigger
        aria-label="Mention a person or an agent"
        disabled={disabled}
        className={cn("[&>svg:last-child]:hidden", className)}
        onKeyDown={onKeyDown}
        render={<Button variant="ghost" size="icon-sm" />}
      >
        <AtSignIcon aria-hidden="true" />
      </ComboboxTrigger>
      <ComboboxContent
        className="min-w-56"
        finalFocus={field}
        initialFocus={!inline}
        onKeyDown={onKeyDown}
      >
        {inline ? null : (
          <ComboboxInput
            aria-label="Search people and agents"
            placeholder="Search people and agents"
            showTrigger={false}
          />
        )}
        <ComboboxEmpty>No one found</ComboboxEmpty>
        <ComboboxList>
          {(option: MentionOption, index: number) => (
            <ComboboxItem
              key={option.value}
              value={option}
              data-active={inline && index === active ? "" : undefined}
              className="data-active:bg-accent data-active:text-accent-foreground"
            >
              <span className="truncate">{option.label}</span>
              {option.hint === null ? null : (
                <span className="ml-auto text-xs text-muted-foreground">
                  {option.hint}
                </span>
              )}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
