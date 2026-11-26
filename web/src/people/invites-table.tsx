import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
} from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { CopyButton } from "@/components/copy-button"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
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
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { roleLabels, type Role } from "@/lib/roles"
import { loadProblemText } from "@/service-accounts/queries"

import {
  invitesQuery,
  inviteProblemText,
  useSaveInvite,
  useWithdrawInvite,
  type Invite,
} from "./invites"
import { neighbourInviteId, withdrawButtonFor } from "./invite-focus"
import { RoleSelect } from "./role-select"

function inviteId(invite: Invite): string {
  return invite.id
}

function InviteRole({ invite }: { invite: Invite }) {
  const save = useSaveInvite()
  const shown = save.isPending ? save.variables.role : invite.role

  function choose(role: Role) {
    if (role === invite.role) return
    save.mutate(
      { email: invite.email, role },
      {
        onSuccess: () =>
          toast.success(`${invite.email} will join as ${roleLabels[role]}`),
        onError: (error) => toast.error(inviteProblemText(error)),
      }
    )
  }

  return (
    <RoleSelect
      size="sm"
      className="w-32"
      aria-label={`Role for ${invite.email}`}
      value={shown}
      disabled={save.isPending}
      onValueChange={choose}
    />
  )
}

const FocusAfterWithdraw = createContext<(id: string | null) => void>(() => {})

function WithdrawButton({ invite }: { invite: Invite }) {
  const [open, setOpen] = useState(false)
  const withdraw = useWithdrawInvite()
  const invites = useQuery(invitesQuery()).data
  const focusAfterWithdraw = useContext(FocusAfterWithdraw)
  const neighbourId = neighbourInviteId(
    (invites ?? []).map(inviteId),
    invite.id
  )

  function confirm() {
    setOpen(false)
    withdraw.mutateAsync(invite.id).then(
      () => {
        toast.success(`Withdrew the invite for ${invite.email}`)
        focusAfterWithdraw(neighbourId)
      },
      (error) => toast.error(inviteProblemText(error))
    )
  }

  return (
    <>
      <Button
        data-withdraw={invite.id}
        variant="outline"
        size="sm"
        className="narrow:h-11"
        disabled={withdraw.isPending}
        aria-busy={withdraw.isPending}
        aria-label={`Withdraw the invite for ${invite.email}`}
        onClick={() => setOpen(true)}
      >
        {withdraw.isPending ? (
          <LoaderCircleIcon
            aria-hidden="true"
            className="motion-safe:animate-spin"
          />
        ) : null}
        Withdraw
      </Button>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent
          finalFocus={() => withdrawButtonFor(neighbourId) ?? true}
        >
          <AlertDialogHeader>
            <AlertDialogTitle>Withdraw this invite?</AlertDialogTitle>
            <AlertDialogDescription>
              {invite.email} will no longer be able to join with this invite.
              You can invite them again later.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirm}>
              Withdraw invite
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

const inviteColumns: DataGridColumn<Invite>[] = [
  {
    id: "email",
    header: "Email",
    minWidth: 200,
    autoHeight: true,
    cell: (invite) => (
      <span className="py-2 font-medium wrap-anywhere">{invite.email}</span>
    ),
  },
  {
    id: "role",
    header: "Role",
    width: 160,
    cell: (invite) => <InviteRole invite={invite} />,
  },
  {
    id: "invitedBy",
    header: "Invited by",
    minWidth: 120,
    cell: (invite) => (
      <span className="truncate text-muted-foreground">
        {invite.invited_by}
      </span>
    ),
  },
  {
    id: "invited",
    header: "Invited",
    width: 120,
    cell: (invite) => (
      <span className="text-muted-foreground">
        <RelativeTime iso={invite.created_at} />
      </span>
    ),
  },
  {
    id: "actions",
    header: "Actions",
    action: true,
    align: "right",
    width: 280,
    cell: (invite) => (
      <span className="inline-flex items-center justify-end gap-2">
        <CopyButton
          text={invite.sign_in_url}
          label={`Copy the sign-in link for ${invite.email}`}
          variant="outline"
          size="sm"
          className="narrow:h-11"
        >
          Copy sign-in link
        </CopyButton>
        <WithdrawButton invite={invite} />
      </span>
    ),
  },
]

export function InvitesTable() {
  const headingId = useId()
  const query = useQuery(invitesQuery())
  const pendingFocus = useRef<string | null>(null)
  const [requests, setRequests] = useState(0)

  function focusAfterWithdraw(inviteId: string | null) {
    pendingFocus.current = inviteId
    setRequests((count) => count + 1)
  }

  useEffect(() => {
    const button = withdrawButtonFor(pendingFocus.current)
    if (button === null) return
    button.focus()
    pendingFocus.current = null
  }, [requests, query.data])

  if (query.isPending) {
    return <Skeleton className="mb-6 h-10 w-full" aria-busy="true" />
  }

  if (query.data === undefined) {
    return (
      <div className="mb-6">
        <LoadProblem
          title="Could not load pending invites"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </div>
    )
  }

  if (query.data.length === 0) return null

  return (
    <section aria-labelledby={headingId} className="mb-6 flex flex-col gap-2">
      <h2 id={headingId} className="text-sm font-semibold">
        Pending invites
      </h2>
      <FocusAfterWithdraw value={focusAfterWithdraw}>
        <DataGrid
          label="Pending invites"
          rows={query.data}
          columns={inviteColumns}
          rowId={inviteId}
          rowAttributes={(invite) => ({ "data-invite-id": invite.id })}
        />
      </FocusAfterWithdraw>
    </section>
  )
}
