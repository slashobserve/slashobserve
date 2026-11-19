import { create } from "zustand"

import { displayZone, isZoneChoice } from "./time"

const defaultTimeZone = "browser"

export const timeZoneKey = "slashobserve-time-zone"

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key)
    else localStorage.setItem(key, value)
  } catch {
    return
  }
}

export function parseStoredTimeZone(raw: string | null): string {
  return raw !== null && isZoneChoice(raw) ? raw : defaultTimeZone
}

type PreferenceState = {
  timeZone: string
  setTimeZone: (zone: string) => void
}

export const usePreferences = create<PreferenceState>((set) => ({
  timeZone: parseStoredTimeZone(read(timeZoneKey)),
  setTimeZone: (zone) => {
    if (!isZoneChoice(zone)) return
    write(timeZoneKey, zone === defaultTimeZone ? null : zone)
    set({ timeZone: zone })
  },
}))

export function preferredZone(zone: string): string | undefined {
  return zone === defaultTimeZone ? undefined : zone
}

export function preferredDisplayZone(zone: string): string | undefined {
  return displayZone(undefined, preferredZone(zone))
}
