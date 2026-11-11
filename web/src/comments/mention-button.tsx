import { useState, type KeyboardEvent, type RefObject } from "react"
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

import { insertMention, type MentionOption } from "./mentions"

type MentionButtonProps = {
  options: readonly MentionOption[]
  field: RefObject<HTMLTextAreaElement | null>
  value: string
  onChange: (next: string) => void
  open: boolean
  onOpenChange: (open: boolean) => void
  replaceFrom: number | null
  disabled?: boolean
  className?: string
}

export function MentionButton({
  options,
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

  function insert(option: MentionOption) {
    const element = field.current
    const start = replaceFrom ?? element?.selectionStart ?? value.length
    const end =
      replaceFrom === null
        ? (element?.selectionEnd ?? value.length)
        : replaceFrom + 1
    const next = insertMention(value, start, end, option.token)
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
