import { TimerIcon } from "lucide-react"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import { useSetDashboardSearch } from "./search"

type Props = { value: string; choices: string[] }

export function RefreshPicker({ value, choices }: Props) {
  const setSearch = useSetDashboardSearch()
  const items = choices.map((choice) => ({
    value: choice,
    label: choice === "off" ? "Off" : choice,
  }))
  const options = items.some((item) => item.value === value)
    ? items
    : [...items, { value, label: value }]
  const label = options.find((item) => item.value === value)?.label ?? value
  return (
    <Select
      items={options}
      value={value}
      onValueChange={(next) => {
        if (next === null) return
        setSearch((state) => ({ ...state, refresh: next }), { replace: true })
      }}
    >
      <SelectTrigger
        aria-label={`Auto-refresh: ${label}`}
        className="min-w-16 data-[size=default]:h-[30px]"
      >
        <TimerIcon aria-hidden="true" className="text-muted-foreground" />
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
