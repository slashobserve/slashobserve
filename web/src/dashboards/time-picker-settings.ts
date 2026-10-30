import {
  liveNowRefresh,
  quickRanges,
  refreshOptions,
  type QuickRange,
} from "@/lib/time"
import type { Dashboard } from "@/spec"

type TimePickerSpec = Pick<Dashboard, "timePicker" | "refresh" | "liveNow">

export function pickerQuickRanges(spec: TimePickerSpec): QuickRange[] {
  const written = spec.timePicker?.quickRanges
  if (!written) return quickRanges
  return written.map((range) => ({
    label: range.display,
    from: range.from,
    to: range.to,
  }))
}

export function pickerRefreshOptions(spec: TimePickerSpec): string[] {
  const written = spec.timePicker?.refreshIntervals
  return written ? ["off", ...written] : refreshOptions
}

export function effectiveRefresh(
  fromUrl: string | undefined,
  spec: TimePickerSpec
): string {
  return fromUrl ?? spec.refresh ?? (spec.liveNow ? liveNowRefresh : "off")
}
