import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { invalidateSession, logout } from "@/lib/auth"

export function useSignOut() {
  const navigate = useNavigate()
  const [pending, setPending] = useState(false)

  async function signOut() {
    setPending(true)
    try {
      await logout()
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
