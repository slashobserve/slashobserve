import { useId, type ReactNode } from "react"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export function TimelineFilters({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-center gap-2 border-b px-4 py-2">{children}</div>
  )
}

type Props<T extends string> = {
  options: readonly { value: T; label: string }[]
  value: T
  onChange: (next: T) => void
}

export function ShowFilter<T extends string>({
  options,
  value,
  onChange,
}: Props<T>) {
  const showId = useId()
  return (
    <>
      <label htmlFor={showId} className="text-xs text-muted-foreground">
        Show
      </label>
      <Select
        items={options}
        value={value}
        onValueChange={(next) => {
          if (next !== null) onChange(next)
        }}
      >
        <SelectTrigger id={showId} size="sm" aria-label="Show">
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </>
  )
}
