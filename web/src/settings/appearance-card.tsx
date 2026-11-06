import { useId } from "react"

import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { isThemePreference, themeChoices, useTheme } from "@/lib/theme"

import { SettingsCard } from "./settings-card"

export function AppearanceCard() {
  const { preference, setPreference } = useTheme()
  const labelId = useId()
  return (
    <SettingsCard
      title="Appearance"
      hint="How the app looks in this browser. The choice stays on this device."
    >
      <div className="flex flex-wrap items-start gap-x-4 gap-y-1 px-4 py-3">
        <span
          id={labelId}
          className="w-[200px] text-[13px] leading-4 text-muted-foreground"
        >
          Theme
        </span>
        <RadioGroup
          aria-labelledby={labelId}
          className="min-w-[200px] flex-1"
          value={preference}
          onValueChange={(value) => {
            if (isThemePreference(value)) setPreference(value)
          }}
        >
          {themeChoices.map((choice) => (
            <label
              key={choice.value}
              className="flex items-center gap-2 text-[13px]"
            >
              <RadioGroupItem value={choice.value} />
              {choice.label}
            </label>
          ))}
        </RadioGroup>
      </div>
    </SettingsCard>
  )
}
