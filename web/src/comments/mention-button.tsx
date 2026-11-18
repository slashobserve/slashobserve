import { useEffect, useState, type KeyboardEvent, type RefObject } from "react"
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
  insertMention,
  matchingOptions,
  mentionOptions,
  peopleOptions,
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
  useEffect(() => {
    const timer = setTimeout(() => setTerm(query.trim()), searchDelayMs)
    return () => clearTimeout(timer)
  }, [query])
  const found = useMentionablePeople(target, term, open)
  const options = offered(agents, people, found, query)

  function insert(option: MentionOption) {
    const element = field.current
    const start = replaceFrom ?? element?.selectionStart ?? value.length
    const end =
      replaceFrom === null
        ? (element?.selectionEnd ?? value.length)
        : replaceFrom + 1
    const next = insertMention(value, start, end, onPick(option))
    onChange(next.value)
    requestAnimationFrame(() => {
      const target = field.current
      if (target === null) return
      target.focus()
      target.setSelectionRange(next.caret, next.caret)
    })
  }

  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === "Escape" && open) event.stopPropagation()
  }

  return (
    <Combobox<MentionOption>
      items={options}
      filter={null}
      value={null}
      open={open}
      inputValue={query}
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
        onKeyDown={onKeyDown}
      >
        <ComboboxInput
          aria-label="Search people and agents"
          placeholder="Search people and agents"
          showTrigger={false}
        />
        <ComboboxEmpty>No one found</ComboboxEmpty>
        <ComboboxList>
          {(option: MentionOption) => (
            <ComboboxItem key={option.value} value={option}>
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
