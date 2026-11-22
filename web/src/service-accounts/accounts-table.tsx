import { useId, useState, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { Cell, HeaderCell, TableCard } from "@/components/table-card"
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
import { Button } from "@/components/ui/button"
import { roleLabels } from "@/lib/roles"

import {
  deleteAccountProblemText,
  deleteAccountText,
  liveKeyCounts,
  useDeleteServiceAccount,
} from "./delete-account"
import type { ServiceAccount } from "./queries"
import { useAllKeys } from "./use-all-keys"

type DeleteProps = {
  account: ServiceAccount
  liveKeys: number
  finalFocus: RefObject<HTMLElement | null>
}

function DeleteAccountButton({ account, liveKeys, finalFocus }: DeleteProps) {
  const [open, setOpen] = useState(false)
  const remove = useDeleteServiceAccount()

  function changeOpen(next: boolean) {
    if (!next && remove.isPending) return
    setOpen(next)
    if (!next) remove.reset()
  }

  function confirm() {
    remove.mutateAsync(account.id).then(
      () => {
        toast.success(`Deleted service account ${account.name}`)
        setOpen(false)
        finalFocus.current?.focus()
      },
      () => {}
    )
  }

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="px-3 font-normal narrow:h-11"
        aria-label={`Delete service account ${account.name}`}
        onClick={() => setOpen(true)}
      >
        Delete
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Delete service account {account.name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {deleteAccountText(account.name, liveKeys)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {remove.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {deleteAccountProblemText(remove.error)}
              </AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              focusableWhenDisabled
              aria-busy={remove.isPending}
              onClick={confirm}
            >
              {remove.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Delete account
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export function AccountsTable({
  deleteFocus,
}: {
  deleteFocus: RefObject<HTMLElement | null>
}) {
  const headingId = useId()
  const keys = useAllKeys()

  if (keys.status !== "success" || keys.accounts.length === 0) return null

  const live = liveKeyCounts(keys.rows.map((row) => row.key))

  return (
    <section aria-labelledby={headingId} className="mt-6 flex flex-col gap-2">
      <div className="flex flex-col">
        <h2 id={headingId} className="text-sm font-semibold">
          Service accounts
        </h2>
        <p className="text-[13px] text-muted-foreground">
          Every key acts as its account. Deleting an account stops all its keys,
          including an account with no keys left.
        </p>
      </div>
      <TableCard labelledBy={headingId} minWidth={560} stackOnNarrow>
        <thead>
          <tr>
            <HeaderCell>Name</HeaderCell>
            <HeaderCell>Role</HeaderCell>
            <HeaderCell>Working keys</HeaderCell>
            <HeaderCell>
              <span className="sr-only">Actions</span>
            </HeaderCell>
          </tr>
        </thead>
        <tbody>
          {keys.accounts.map((account) => (
            <tr key={account.id} data-account-id={account.id}>
              <Cell>
                <div className="font-medium wrap-anywhere">{account.name}</div>
                {account.description === "" ? null : (
                  <div className="text-xs wrap-anywhere text-muted-foreground">
                    {account.description}
                  </div>
                )}
              </Cell>
              <Cell data-label="Role" className="text-muted-foreground">
                {roleLabels[account.role]}
              </Cell>
              <Cell data-label="Working keys" className="text-muted-foreground">
                {live.get(account.id) ?? 0}
              </Cell>
              <Cell data-cell="action" className="text-right">
                {account.provisioned ? (
                  <span className="text-xs text-muted-foreground">
                    From the service-accounts file
                  </span>
                ) : (
                  <DeleteAccountButton
                    account={account}
                    liveKeys={live.get(account.id) ?? 0}
                    finalFocus={deleteFocus}
                  />
                )}
              </Cell>
            </tr>
          ))}
        </tbody>
      </TableCard>
    </section>
  )
}
