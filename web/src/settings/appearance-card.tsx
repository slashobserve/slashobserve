import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import {
  isWeekStart,
  usePreferences,
  weekStartChoices,
} from "@/lib/preferences"
import { isThemePreference, themeChoices, useTheme } from "@/lib/theme"
import { zoneChoiceLabel, zoneChoices } from "@/lib/time"

import { SettingsCard } from "./settings-card"
import { SettingsSection } from "./settings-section"

const radioRowClass =
  "flex min-h-9 cursor-pointer items-center gap-2.5 px-4 text-[13px] hover:bg-canvas narrow:min-h-11"

export function AppearanceSection() {
  const { preference, setPreference } = useTheme()
  const { timeZone, setTimeZone, weekStart, setWeekStart } = usePreferences()
  const zones = zoneChoices()
  return (
    <SettingsSection
      title="Appearance"
      description="How the app looks and reads times in this browser. The choices stay on this device."
    >
      <SettingsCard
        title="Theme"
        hint="Kept in this browser only, nothing is saved on the server."
      >
        <RadioGroup
          aria-label="Theme"
          className="gap-0 divide-y"
          value={preference}
          onValueChange={(value) => {
            if (isThemePreference(value)) setPreference(value)
          }}
        >
          {themeChoices.map((choice) => (
            <label key={choice.value} className={radioRowClass}>
              <RadioGroupItem value={choice.value} />
              {choice.label}
            </label>
          ))}
        </RadioGroup>
      </SettingsCard>
      <SettingsCard
        title="Time zone"
        hint="The zone a dashboard shows times in when it does not set one, and the zone typed times in the time range box are read in. Kept in this browser only."
      >
        <div className="px-4 py-3">
          <Combobox
            items={zones}
            value={timeZone}
            itemToStringLabel={zoneChoiceLabel}
            onValueChange={(next) => {
              if (next !== null) setTimeZone(next)
            }}
          >
            <ComboboxInput
              aria-label="Time zone"
              placeholder="Search time zones"
              className="w-72 max-w-full"
            />
            <ComboboxContent align="start" className="min-w-56">
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
        </div>
      </SettingsCard>
      <SettingsCard
        title="Week starts on"
        hint="The first day of the week for ranges you move or zoom on a dashboard that does not set one. Kept in this browser only."
      >
        <RadioGroup
          aria-label="Week starts on"
          className="gap-0 divide-y"
          value={weekStart}
          onValueChange={(value) => {
            if (isWeekStart(value)) setWeekStart(value)
          }}
        >
          {weekStartChoices.map((choice) => (
            <label key={choice.value} className={radioRowClass}>
              <RadioGroupItem value={choice.value} />
              {choice.label}
            </label>
          ))}
        </RadioGroup>
      </SettingsCard>
    </SettingsSection>
  )
}
