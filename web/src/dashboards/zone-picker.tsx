import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { zoneChoiceLabel, zoneChoices } from "@/lib/time"

import { useSetDashboardSearch } from "./search"

type Props = { value: string; dashboardZone: string | undefined }

export function ZonePicker({ value, dashboardZone }: Props) {
  const setSearch = useSetDashboardSearch()
  const choices = zoneChoices()
  const items = choices.includes(value) ? choices : [value, ...choices]
  return (
    <Combobox
      items={items}
      value={value}
      itemToStringLabel={zoneChoiceLabel}
      onValueChange={(next) => {
        if (next === null) return
        const own = dashboardZone ?? "browser"
        setSearch(
          (state) => ({ ...state, tz: next === own ? undefined : next }),
          { replace: true }
        )
      }}
    >
      <ComboboxInput
        aria-label={`Time zone: ${zoneChoiceLabel(value)}`}
        placeholder="Search time zones"
        className="h-[30px] w-40"
      />
      <ComboboxContent align="end" className="min-w-56">
        <ComboboxEmpty>No time zones found</ComboboxEmpty>
        <ComboboxList>
          {(zone: string) => (
            <ComboboxItem key={zone} value={zone}>
              {zoneChoiceLabel(zone)}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
