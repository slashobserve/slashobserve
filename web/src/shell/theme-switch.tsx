import { MoonIcon, SunIcon, SunMoonIcon } from "lucide-react"

import {
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@/components/ui/dropdown-menu"
import { isThemePreference, themeChoices, useTheme } from "@/lib/theme"

export function useToggleTheme() {
  const { resolved, setPreference } = useTheme()
  return () => setPreference(resolved === "dark" ? "light" : "dark")
}

export function ThemeSwitch() {
  const { resolved } = useTheme()
  const toggle = useToggleTheme()
  const Icon = resolved === "dark" ? SunIcon : MoonIcon
  return (
    <button
      type="button"
      aria-label="Switch theme"
      onClick={toggle}
      className="grid size-[30px] place-items-center rounded-control text-foreground outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
    >
      <Icon aria-hidden="true" className="size-4" strokeWidth={2} />
    </button>
  )
}

export function ThemeMenu() {
  const { preference, setPreference } = useTheme()
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger>
        <SunMoonIcon aria-hidden="true" />
        Theme
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="min-w-max">
        <DropdownMenuRadioGroup
          value={preference}
          onValueChange={(value) => {
            if (isThemePreference(value)) setPreference(value)
          }}
        >
          {themeChoices.map((choice) => (
            <DropdownMenuRadioItem
              key={choice.value}
              value={choice.value}
              closeOnClick
            >
              {choice.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  )
}
