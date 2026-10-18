import { MoonIcon, SunIcon } from "lucide-react"

import { useTheme } from "@/lib/theme"

export function ThemeSwitch() {
  const { resolved, setPreference } = useTheme()
  const Icon = resolved === "dark" ? SunIcon : MoonIcon
  return (
    <button
      type="button"
      aria-label="Switch theme"
      onClick={() => setPreference(resolved === "dark" ? "light" : "dark")}
      className="grid size-[30px] place-items-center rounded-control text-foreground outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
    >
      <Icon aria-hidden="true" className="size-4" strokeWidth={2} />
    </button>
  )
}
