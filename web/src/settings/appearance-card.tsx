import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { isThemePreference, themeChoices, useTheme } from "@/lib/theme"

import { SettingsCard } from "./settings-card"
import { SettingsSection } from "./settings-section"

export function AppearanceSection() {
  const { preference, setPreference } = useTheme()
  return (
    <SettingsSection
      title="Appearance"
      description="How the app looks in this browser. The choice stays on this device."
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
            <label
              key={choice.value}
              className="flex min-h-9 cursor-pointer items-center gap-2.5 px-4 text-[13px] hover:bg-canvas narrow:min-h-11"
            >
              <RadioGroupItem value={choice.value} />
              {choice.label}
            </label>
          ))}
        </RadioGroup>
      </SettingsCard>
    </SettingsSection>
  )
}
