import { createContext, useContext } from "react"

import type { CanvasActionContext } from "./actions"

const ActionContext = createContext<CanvasActionContext | null>(null)

export const CanvasActionProvider = ActionContext.Provider

export function useCanvasActions(): CanvasActionContext | null {
  return useContext(ActionContext)
}
