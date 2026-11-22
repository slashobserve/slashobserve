import { useContext, useLayoutEffect, type ReactNode } from "react"
import { useRouteContext } from "@tanstack/react-router"

import { RoleNote } from "@/components/role-note"
import { roleAtLeast, roleLabels, type Role } from "@/lib/roles"
import { HideCrumbContext } from "@/shell/crumbs"

function CrumblessNote({ children }: { children: ReactNode }) {
  const setHidden = useContext(HideCrumbContext)
  useLayoutEffect(() => {
    setHidden(true)
    return () => setHidden(false)
  }, [setHidden])
  return <RoleNote className="py-2">{children}</RoleNote>
}

export function RoleGate({
  min,
  note,
  hideCrumb = false,
  children,
}: {
  min: Role
  note?: string
  hideCrumb?: boolean
  children: ReactNode
}) {
  const { principal } = useRouteContext({ from: "/_authed" })
  if (roleAtLeast(principal.role, min)) return children
  const text = note ?? `Only ${roleLabels[min]}s can open this page.`
  if (hideCrumb) return <CrumblessNote>{text}</CrumblessNote>
  return <RoleNote className="py-2">{text}</RoleNote>
}
