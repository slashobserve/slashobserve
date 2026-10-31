import { createContext, useContext } from "react"

import type { ActionContext } from "./actions"

const Context = createContext<ActionContext | null>(null)

export const ActionProvider = Context.Provider

export function useActionContext(): ActionContext | null {
  return useContext(Context)
}
