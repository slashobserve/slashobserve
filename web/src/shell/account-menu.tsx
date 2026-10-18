import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { LogOutIcon } from "lucide-react"

import { instanceQuery } from "@/api/instance"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

import { initials } from "./initials"
import { useSignOut } from "./use-sign-out"

export function AccountMenu({ collapsed }: { collapsed: boolean }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const { signOut, pending } = useSignOut()
  const label = principal.email || principal.name
  const instance = useQuery(instanceQuery())
  const providerName = instance.data?.oidc_provider_name.trim()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            aria-label={`Account: ${[principal.name, principal.email].filter(Boolean).join(", ")}`}
            title={label}
            className={cn(
              "flex min-h-10 items-center gap-2.5 rounded-[7px] px-1.5 text-left outline-none hover:bg-background focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-background",
              collapsed && "justify-center px-0"
            )}
          />
        }
      >
        <span
          aria-hidden="true"
          className="grid size-[26px] flex-none place-items-center rounded-full border bg-background text-[11px] font-semibold"
        >
          {initials(principal.name, principal.email)}
        </span>
        {collapsed ? null : (
          <span className="flex min-w-0 flex-col leading-[1.25]">
            <span className="truncate text-[13px]">{label}</span>
            {providerName ? (
              <span className="truncate text-xs text-muted-foreground">
                Signed in with {providerName}
              </span>
            ) : null}
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start">
        <DropdownMenuItem disabled={pending} onClick={signOut}>
          <LogOutIcon aria-hidden="true" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
