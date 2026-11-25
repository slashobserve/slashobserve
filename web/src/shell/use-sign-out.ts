import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { useActivityStore } from "@/activity/activity-store"
import { useInboxPanelStore } from "@/inbox/inbox-panel-store"
import { resetLiveToasts } from "@/inbox/use-live-toasts"
import { invalidateSession, logout } from "@/lib/auth"

export function useSignOut() {
  const navigate = useNavigate()
  const [pending, setPending] = useState(false)

  async function signOut() {
    setPending(true)
    try {
      await logout()
      useActivityStore.getState().forgetChats()
      useInboxPanelStore.setState(useInboxPanelStore.getInitialState())
      resetLiveToasts()
      invalidateSession()
      await navigate({ to: "/login" })
    } catch {
      toast.error("Sign out failed. Try again.")
    } finally {
      setPending(false)
    }
  }

  return { signOut, pending }
}
