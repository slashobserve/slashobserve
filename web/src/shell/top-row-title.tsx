import { createContext, useContext, type ReactNode } from "react"
import { createPortal } from "react-dom"

export const TitleHostContext = createContext<HTMLElement | null>(null)

export function TopRowTitle({ children }: { children: ReactNode }) {
  const host = useContext(TitleHostContext)
  if (!host) return null
  return createPortal(children, host)
}
