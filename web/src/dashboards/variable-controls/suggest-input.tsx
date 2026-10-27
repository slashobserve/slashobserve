import { useState } from "react"
import { Autocomplete } from "@base-ui/react/autocomplete"

import { Input } from "@/components/ui/input"

type Props = {
  id: string
  value: string
  suggestions: string[]
  onChange: (value: string) => void
}

export function SuggestInput({ id, value, suggestions, onChange }: Props) {
  const [open, setOpen] = useState(false)
  const typed = value.toLowerCase()
  const hasMatch = suggestions.some((item) =>
    item.toLowerCase().includes(typed)
  )
  return (
    <Autocomplete.Root
      items={suggestions}
      value={value}
      onValueChange={onChange}
      open={open && hasMatch}
      onOpenChange={setOpen}
    >
      <Autocomplete.Input id={id} render={<Input />} />
      <Autocomplete.Portal>
        <Autocomplete.Positioner
          sideOffset={4}
          align="start"
          className="isolate z-50"
        >
          <Autocomplete.Popup className="max-h-(--available-height) w-(--anchor-width) overflow-hidden rounded-lg border border-border bg-popover text-popover-foreground">
            <Autocomplete.List className="no-scrollbar max-h-60 overflow-y-auto overscroll-contain p-1">
              {(item: string) => (
                <Autocomplete.Item
                  key={item}
                  value={item}
                  className="flex w-full cursor-default items-center rounded-md px-1.5 py-1 text-[13px] outline-hidden select-none data-highlighted:bg-accent data-highlighted:text-accent-foreground"
                >
                  {item}
                </Autocomplete.Item>
              )}
            </Autocomplete.List>
          </Autocomplete.Popup>
        </Autocomplete.Positioner>
      </Autocomplete.Portal>
    </Autocomplete.Root>
  )
}
