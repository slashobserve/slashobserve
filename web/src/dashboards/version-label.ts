import type { LiveStatus } from "./live/use-dashboard-events"

type Input = {
  version: number
  latest: number
  pinned: boolean
  status: LiveStatus
}

const statusLabels: Record<LiveStatus, string> = {
  live: "Live",
  connecting: "Connecting",
  reconnecting: "Reconnecting",
}

export function isPinnedOlder({ version, latest, pinned }: Input): boolean {
  return pinned && version < latest
}

export function versionButtonText(input: Input): string {
  if (isPinnedOlder(input)) return `v${input.version} of ${input.latest}`
  return `${statusLabels[input.status]} · v${input.version}`
}

export function versionButtonLabel(input: Input): string {
  if (isPinnedOlder(input)) {
    return `Version ${input.version} of ${input.latest}. Choose a version`
  }
  return `Version ${input.version}, ${input.status}. Choose a version`
}
