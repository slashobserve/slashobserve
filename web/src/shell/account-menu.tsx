import { useId } from "react"
import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { LogOutIcon } from "lucide-react"

import { instanceQuery } from "@/api/instance"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { AuthorMark } from "@/components/author-mark"
import { personTint } from "@/lib/author"
import { cn } from "@/lib/utils"
import { providerDisplayName } from "@/people/sign-in-policy"

import { initials } from "./initials"
import { ThemeMenu } from "./theme-switch"
import { useSignOut } from "./use-sign-out"

export function AccountMenu({ collapsed }: { collapsed: boolean }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const { signOut, pending } = useSignOut()
  const label = principal.email || principal.name
  const instance = useQuery(instanceQuery())
  const providerName = providerDisplayName(instance.data?.oidc_provider_name)
  const providerId = useId()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            aria-describedby={
              !collapsed && providerName ? providerId : undefined
            }
            aria-label={`Account: ${[principal.name, principal.email].filter(Boolean).join(", ")}`}
            title={label}
            className={cn(
              "rounded-control outline-none hover:bg-nav-active focus-visible:ring-2 focus-visible:ring-ring aria-expanded:bg-nav-active",
              collapsed
                ? "grid size-9 place-items-center p-0"
                : "flex min-h-10 min-w-0 flex-1 items-center gap-2.5 px-1.5 text-left"
            )}
          />
        }
      >
        <AuthorMark
          mark={{
            kind: "person",
            text: initials(principal.name, principal.email),
            tint: personTint(principal.id),
          }}
          size={24}
        />
        {collapsed ? null : (
          <span className="flex min-w-0 flex-col leading-[1.25]">
            <span className="truncate text-[13px]">{label}</span>
            {providerName ? (
              <span
                id={providerId}
                className="truncate text-xs text-muted-foreground"
              >
                Signed in with {providerName}
              </span>
            ) : null}
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start">
        <ThemeMenu />
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled={pending} onClick={signOut}>
          <LogOutIcon aria-hidden="true" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
