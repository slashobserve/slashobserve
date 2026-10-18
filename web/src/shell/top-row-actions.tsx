import { createContext, useContext, type ReactNode } from "react"
import { createPortal } from "react-dom"

export const ActionsHostContext = createContext<HTMLElement | null>(null)

export function TopRowActions({ children }: { children: ReactNode }) {
  const host = useContext(ActionsHostContext)
  if (!host) return null
  return createPortal(children, host)
}
