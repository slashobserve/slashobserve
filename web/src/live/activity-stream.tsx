import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react"

import {
  matchesActivityFilter,
  type ActivityEvent,
  type ActivityFilter,
} from "./activity-match"
import { useActivityStream, useDocumentVisible } from "./use-activity-stream"

type ActivityListener = {
  onEvent: (event: ActivityEvent) => void
  onGap: () => void
}

type ActivityHub = ActivityListener & {
  subscribe: (listener: ActivityListener) => () => void
}

function createActivityHub(): ActivityHub {
  const listeners = new Set<ActivityListener>()
  return {
    subscribe: (listener) => {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    onEvent: (event) => {
      for (const listener of [...listeners]) listener.onEvent(event)
    },
    onGap: () => {
      for (const listener of [...listeners]) listener.onGap()
    },
  }
}

const ActivityHubContext = createContext<ActivityHub | null>(null)

export function ActivityStreamProvider({ children }: { children: ReactNode }) {
  const [hub] = useState(createActivityHub)
  useActivityStream({
    enabled: useDocumentVisible(),
    onEvent: hub.onEvent,
    onGap: hub.onGap,
  })
  return <ActivityHubContext value={hub}>{children}</ActivityHubContext>
}

export function useHasActivityStream(): boolean {
  return useContext(ActivityHubContext) !== null
}

export function useActivitySubscription(
  subscription: ActivityFilter & ActivityListener
): void {
  const hub = useContext(ActivityHubContext)
  const latest = useRef(subscription)
  useEffect(() => {
    latest.current = subscription
  })
  useEffect(
    () =>
      hub?.subscribe({
        onEvent: (event) => {
          if (matchesActivityFilter(event, latest.current)) {
            latest.current.onEvent(event)
          }
        },
        onGap: () => latest.current.onGap(),
      }),
    [hub]
  )
}
