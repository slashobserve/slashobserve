import { createContext, useContext } from "react"

const CloseControls = createContext<() => void>(() => {})

export const CloseControlsProvider = CloseControls.Provider

export function useCloseControls(): () => void {
  return useContext(CloseControls)
}
