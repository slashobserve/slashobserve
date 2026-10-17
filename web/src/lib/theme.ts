import { create } from "zustand"

export type ThemePreference = "light" | "dark" | "system"
export type ResolvedTheme = "light" | "dark"

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
  setPreference: (preference: ThemePreference) => void
}

const useThemeStore = create<ThemeState>((set, get) => ({
  preference: readPreference(),
  systemDark: media.matches,
  setPreference: (preference) => {
    writePreference(preference)
    set({ preference })
    apply(resolve(preference, get().systemDark))
  },
}))

media.addEventListener("change", (event) => {
  useThemeStore.setState({ systemDark: event.matches })
  apply(resolve(useThemeStore.getState().preference, event.matches))
})

export function resolvedTheme(): ResolvedTheme {
  const { preference, systemDark } = useThemeStore.getState()
  return resolve(preference, systemDark)
}

export function useTheme() {
  const preference = useThemeStore((state) => state.preference)
  const systemDark = useThemeStore((state) => state.systemDark)
  const setPreference = useThemeStore((state) => state.setPreference)
  return {
    preference,
    resolved: resolve(preference, systemDark),
    setPreference,
  }
}
