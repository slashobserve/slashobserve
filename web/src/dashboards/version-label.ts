import type { LiveStatus } from "./live/use-dashboard-events"

type Input = {
  version: number
  latest: number
  status: LiveStatus
}

const statusLabels: Record<LiveStatus, string> = {
  live: "Live",
  connecting: "Connecting",
  reconnecting: "Reconnecting",
}

export function isOlderVersion({ version, latest }: Input): boolean {
  return version < latest
}

export function versionButtonText(input: Input): string {
  if (isOlderVersion(input)) return `v${input.version} of ${input.latest}`
  return `${statusLabels[input.status]} · v${input.version}`
}

export function versionButtonLabel(input: Input): string {
  return `${versionButtonText(input)}. Choose a version`
}
