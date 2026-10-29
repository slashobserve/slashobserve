import type { ReactNode } from "react"
import { useRouteContext } from "@tanstack/react-router"

import { RoleNote } from "@/components/role-note"
import { roleAtLeast, roleLabels, type Role } from "@/lib/roles"

export function RoleGate({
  min,
  note,
  children,
}: {
  min: Role
  note?: string
  children: ReactNode
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  if (roleAtLeast(principal.role, min)) return children
  return (
    <RoleNote className="py-2">
      {note ?? `Only ${roleLabels[min]}s can open this page.`}
    </RoleNote>
  )
}
