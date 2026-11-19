import { create } from "zustand"

export type ThemePreference = "light" | "dark" | "system"
export type ResolvedTheme = "light" | "dark"

export const themeChoices: readonly {
  value: ThemePreference
  label: string
}[] = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "system", label: "Match the system" },
]

export function isThemePreference(value: unknown): value is ThemePreference {
  return themeChoices.some((choice) => choice.value === value)
}

const storageKey = "slashobserve-theme"

function readPreference(): ThemePreference {
  try {
    const stored = localStorage.getItem(storageKey)
    return stored === "light" || stored === "dark" ? stored : "system"
  } catch {
    return "system"
  }
}

function writePreference(preference: ThemePreference) {
  try {
    if (preference === "system") localStorage.removeItem(storageKey)
    else localStorage.setItem(storageKey, preference)
  } catch {
    return
  }
}

function resolve(
  preference: ThemePreference,
  systemDark: boolean
): ResolvedTheme {
  if (preference === "system") return systemDark ? "dark" : "light"
  return preference
}

function apply(resolved: ResolvedTheme) {
  document.documentElement.classList.toggle("dark", resolved === "dark")
  document.documentElement.style.colorScheme = resolved
}

const media = window.matchMedia("(prefers-color-scheme: dark)")

type ThemeState = {
  preference: ThemePreference
  systemDark: boolean
  held: ResolvedTheme | null
  setPreference: (preference: ThemePreference) => void
}

function shown(state: ThemeState): ResolvedTheme {
  return state.held ?? resolve(state.preference, state.systemDark)
}

const useThemeStore = create<ThemeState>((set, get) => ({
  preference: readPreference(),
  systemDark: media.matches,
  held: null,
  setPreference: (preference) => {
    writePreference(preference)
    set({ preference })
    apply(shown(get()))
  },
}))

function onMediaChange(event: MediaQueryListEvent) {
  useThemeStore.setState({ systemDark: event.matches })
  apply(shown(useThemeStore.getState()))
}

media.addEventListener("change", onMediaChange)

function onStorage(event: StorageEvent) {
  if (event.storageArea !== localStorage) return
  if (event.key !== null && event.key !== storageKey) return
  const preference = readPreference()
  useThemeStore.setState({ preference })
  apply(shown(useThemeStore.getState()))
}

window.addEventListener("storage", onStorage)
import.meta.hot?.dispose(() => {
  window.removeEventListener("storage", onStorage)
  media.removeEventListener("change", onMediaChange)
})

export function holdLightTheme(): () => void {
  useThemeStore.setState({ held: "light" })
  apply("light")
  return () => {
    useThemeStore.setState({ held: null })
    apply(shown(useThemeStore.getState()))
  }
}

export function useTheme() {
  const preference = useThemeStore((state) => state.preference)
  const systemDark = useThemeStore((state) => state.systemDark)
  const held = useThemeStore((state) => state.held)
  const setPreference = useThemeStore((state) => state.setPreference)
  return {
    preference,
    resolved: held ?? resolve(preference, systemDark),
    setPreference,
  }
}
