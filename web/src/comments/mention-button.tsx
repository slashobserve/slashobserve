import { useRef, useState, type KeyboardEvent, type RefObject } from "react"

import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { cn } from "@/lib/utils"

import { insertMention, mentionToken, type MentionPerson } from "./mentions"

type Option = { value: string; label: string; token: string; agent: boolean }

const observeOption: Option = {
  value: "agent:observe",
  label: "@observe",
  token: "@observe",
  agent: true,
}

function optionsFor(people: readonly MentionPerson[]): Option[] {
  return [
    observeOption,
    ...people.map((person) => ({
      value: `user:${person.id}`,
      label: person.name === "" ? "Unnamed" : person.name,
      token: mentionToken(person),
      agent: false,
    })),
  ]
}

export function MentionButton({
  people,
  field,
  value,
  onChange,
  disabled,
  className,
}: {
  people: readonly MentionPerson[]
  field: RefObject<HTMLTextAreaElement | null>
  value: string
  onChange: (next: string) => void
  disabled?: boolean
  className?: string
}) {
  const [query, setQuery] = useState("")
  const open = useRef(false)

  function insert(option: Option) {
    const element = field.current
    const start = element?.selectionStart ?? value.length
    const end = element?.selectionEnd ?? value.length
    const next = insertMention(value, start, end, option.token)
    onChange(next.value)
    requestAnimationFrame(() => {
      const target = field.current
      if (target === null) return
      target.focus()
      target.setSelectionRange(next.caret, next.caret)
    })
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape" && open.current) event.stopPropagation()
  }

  return (
    <div className={cn("w-32", className)} onKeyDown={onKeyDown}>
      <Combobox<Option>
        items={optionsFor(people)}
        value={null}
        inputValue={query}
        disabled={disabled}
        itemToStringLabel={(option) => option.label}
        onOpenChange={(next) => {
          open.current = next
        }}
        onInputValueChange={(text, details) =>
          setQuery(details.reason === "item-press" ? "" : text)
        }
        onValueChange={(next) => {
          if (next !== null) insert(next)
        }}
      >
        <ComboboxInput
          aria-label="Mention"
          placeholder="Mention"
          disabled={disabled}
          className="h-7 w-full"
        />
        <ComboboxContent className="min-w-56">
          <ComboboxEmpty>No one found</ComboboxEmpty>
          <ComboboxList>
            {(option: Option) => (
              <ComboboxItem key={option.value} value={option}>
                <span className="truncate">{option.label}</span>
                {option.agent ? (
                  <span className="ml-auto text-xs text-muted-foreground">
                    Agent
                  </span>
                ) : null}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </div>
  )
}
