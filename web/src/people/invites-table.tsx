import { useId } from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { CopyButton } from "@/components/copy-button"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
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
import { RoleSelect } from "./role-select"

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

function WithdrawButton({ invite }: { invite: Invite }) {
  const withdraw = useWithdrawInvite()
  return (
    <Button
      variant="outline"
      size="sm"
      className="narrow:h-11"
      disabled={withdraw.isPending}
      aria-busy={withdraw.isPending}
      aria-label={`Withdraw the invite for ${invite.email}`}
      onClick={() =>
        withdraw.mutateAsync(invite.id).then(
          () => toast.success(`Withdrew the invite for ${invite.email}`),
          (error) => toast.error(inviteProblemText(error))
        )
      }
    >
      {withdraw.isPending ? (
        <LoaderCircleIcon
          aria-hidden="true"
          className="motion-safe:animate-spin"
        />
      ) : null}
      Withdraw
    </Button>
  )
}

export function InvitesTable() {
  const headingId = useId()
  const query = useQuery(invitesQuery())

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
      <TableCard labelledBy={headingId} minWidth={760} stackOnNarrow>
        <thead>
          <tr>
            <HeaderCell>Email</HeaderCell>
            <HeaderCell>Role</HeaderCell>
            <HeaderCell>Invited by</HeaderCell>
            <HeaderCell>Invited</HeaderCell>
            <HeaderCell>
              <span className="sr-only">Actions</span>
            </HeaderCell>
          </tr>
        </thead>
        <tbody>
          {query.data.map((invite) => (
            <tr key={invite.id} data-invite-id={invite.id}>
              <Cell className="font-medium wrap-anywhere">{invite.email}</Cell>
              <Cell>
                <InviteRole invite={invite} />
              </Cell>
              <Cell data-label="Invited by" className="text-muted-foreground">
                {invite.invited_by}
              </Cell>
              <Cell data-label="Invited" className="text-muted-foreground">
                <RelativeTime iso={invite.created_at} />
              </Cell>
              <Cell data-cell="action" className="text-right">
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
              </Cell>
            </tr>
          ))}
        </tbody>
      </TableCard>
    </section>
  )
}
