import { create } from "zustand"

import type { Dashboard } from "@/spec"

import { displayZone, isZoneChoice } from "./time"

export type WeekStart = NonNullable<Dashboard["weekStart"]>

export const weekStartChoices: readonly { value: WeekStart; label: string }[] =
  [
    { value: "monday", label: "Monday" },
    { value: "sunday", label: "Sunday" },
    { value: "saturday", label: "Saturday" },
  ]

const defaultTimeZone = "browser"
const defaultWeekStart: WeekStart = "monday"

export const timeZoneKey = "slashobserve-time-zone"
export const weekStartKey = "slashobserve-week-start"

export function isWeekStart(value: unknown): value is WeekStart {
  return weekStartChoices.some((choice) => choice.value === value)
}

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

export function parseStoredWeekStart(raw: string | null): WeekStart {
  return isWeekStart(raw) ? raw : defaultWeekStart
}

type PreferenceState = {
  timeZone: string
  weekStart: WeekStart
  setTimeZone: (zone: string) => void
  setWeekStart: (day: WeekStart) => void
}

export const usePreferences = create<PreferenceState>((set) => ({
  timeZone: parseStoredTimeZone(read(timeZoneKey)),
  weekStart: parseStoredWeekStart(read(weekStartKey)),
  setTimeZone: (zone) => {
    if (!isZoneChoice(zone)) return
    write(timeZoneKey, zone === defaultTimeZone ? null : zone)
    set({ timeZone: zone })
  },
  setWeekStart: (day) => {
    write(weekStartKey, day === defaultWeekStart ? null : day)
    set({ weekStart: day })
  },
}))

export function preferredZone(zone: string): string | undefined {
  return zone === defaultTimeZone ? undefined : zone
}

export function preferredDisplayZone(zone: string): string | undefined {
  return displayZone(undefined, preferredZone(zone))
}
