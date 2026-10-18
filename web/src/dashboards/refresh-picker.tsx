import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { refreshOptions } from "@/lib/time"

import { useSetDashboardSearch } from "./search"

const items = refreshOptions.map((value) => ({
  value,
  label: value === "off" ? "Off" : value,
}))

type Props = { value: string }

export function RefreshPicker({ value }: Props) {
  const setSearch = useSetDashboardSearch()
  const options = items.some((item) => item.value === value)
    ? items
    : [...items, { value, label: value }]
  return (
    <Select
      items={options}
      value={value}
      onValueChange={(next) => {
        if (next === null) return
        setSearch((state) => ({ ...state, refresh: next }), { replace: true })
      }}
    >
      <SelectTrigger aria-label="Auto-refresh" className="min-w-24">
        <span className="hidden text-muted-foreground sm:inline">
          Auto-refresh
        </span>
        <SelectValue />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="end">
        {options.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
