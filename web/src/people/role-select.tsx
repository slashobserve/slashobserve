import { useRef, useState } from "react"
import { useNavigate, useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon, LockIcon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { refreshSession } from "@/lib/auth"
import { roleDescriptions, roleLabels, roles, type Role } from "@/lib/roles"

import { setRoleProblemText, useSetRole, type User } from "./queries"
import { roleSourceHint } from "./role-source"

const roleItems = roles.map((role) => ({
  value: role,
  label: roleLabels[role],
}))

export function RoleSelect({
  value,
  onValueChange,
  disabled,
  id,
  size,
  className,
  "aria-label": ariaLabel,
  triggerRef,
}: {
  value: Role
  onValueChange: (role: Role) => void
  disabled?: boolean
  id?: string
  size?: "sm" | "default"
  className?: string
  "aria-label"?: string
  triggerRef?: React.Ref<HTMLButtonElement>
}) {
  return (
    <Select
      items={roleItems}
      value={value}
      disabled={disabled}
      onValueChange={(next) => {
        if (next !== null) onValueChange(next)
      }}
    >
      <SelectTrigger
        ref={triggerRef}
        id={id}
        size={size}
        aria-label={ariaLabel}
        className={className}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        alignItemWithTrigger={false}
        align="start"
        className="w-72 max-w-[calc(100vw-2rem)]"
      >
        {roles.map((role) => (
          <SelectItem key={role} value={role}>
            <span className="flex flex-col py-0.5">
              <span>{roleLabels[role]}</span>
              <span className="text-xs whitespace-normal text-muted-foreground">
                {roleDescriptions[role]}
              </span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export function PersonRole({ person }: { person: User }) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const navigate = useNavigate()
  const setRole = useSetRole()
  const trigger = useRef<HTMLButtonElement>(null)
  const [confirming, setConfirming] = useState<Role | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const isMe = principal.id === person.id
  const shown = setRole.isPending ? setRole.variables.role : person.role

  function apply(role: Role) {
    setProblem(null)
    setRole.mutate(
      { id: person.id, role },
      {
        onSuccess: () => {
          setConfirming(null)
          if (isMe) {
            void refreshSession()
              .catch(() => undefined)
              .then(() => navigate({ to: "/" }))
            return
          }
          toast.success(`${person.name} is now ${roleLabels[role]}`)
        },
        onError: (error) => {
          const text = setRoleProblemText(error)
          if (isMe) setProblem(text)
          else toast.error(text)
        },
      }
    )
  }

  function choose(role: Role) {
    if (role === person.role) return
    if (isMe) {
      setConfirming(role)
      return
    }
    apply(role)
  }

  function changeOpen(open: boolean) {
    if (open || setRole.isPending) return
    setConfirming(null)
    setProblem(null)
    setRole.reset()
  }

  return (
    <span className="flex flex-col items-start gap-0.5">
      {person.role_locked ? (
        <span className="inline-flex min-h-7 items-center gap-1.5">
          <LockIcon aria-hidden="true" className="size-3 flex-none" />
          {roleLabels[person.role]}
        </span>
      ) : (
        <RoleSelect
          triggerRef={trigger}
          size="sm"
          className="w-32"
          aria-label={`Role for ${person.name}`}
          value={shown}
          disabled={setRole.isPending}
          onValueChange={choose}
        />
      )}
      <span className="text-xs text-muted-foreground">
        {roleSourceHint(person)}
      </span>
      <AlertDialog open={confirming !== null} onOpenChange={changeOpen}>
        <AlertDialogContent finalFocus={trigger}>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Change your own role to {confirming && roleLabels[confirming]}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              You will lose access to this page if it is lower than Admin.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={setRole.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={setRole.isPending}
              focusableWhenDisabled
              aria-busy={setRole.isPending}
              onClick={() => confirming !== null && apply(confirming)}
            >
              {setRole.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Change role
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </span>
  )
}
