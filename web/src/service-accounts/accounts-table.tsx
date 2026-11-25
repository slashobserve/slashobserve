import { useId, useState, type RefObject } from "react"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
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

function accountId(account: ServiceAccount): string {
  return account.id
}

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

function accountColumns(
  live: ReadonlyMap<string, number>,
  deleteFocus: RefObject<HTMLElement | null>
): DataGridColumn<ServiceAccount>[] {
  return [
    {
      id: "name",
      header: "Name",
      minWidth: 200,
      flex: 2,
      autoHeight: true,
      cell: (account) => (
        <div className="py-2">
          <div className="font-medium wrap-anywhere">{account.name}</div>
          {account.description === "" ? null : (
            <div className="text-xs wrap-anywhere text-muted-foreground">
              {account.description}
            </div>
          )}
        </div>
      ),
    },
    {
      id: "role",
      header: "Role",
      width: 120,
      cell: (account) => (
        <span className="text-muted-foreground">
          {roleLabels[account.role]}
        </span>
      ),
    },
    {
      id: "workingKeys",
      header: "Working keys",
      width: 140,
      cell: (account) => (
        <span className="text-muted-foreground">
          {live.get(account.id) ?? 0}
        </span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      width: 220,
      cell: (account) =>
        account.provisioned ? (
          <span className="text-xs text-muted-foreground">
            From the service-accounts file
          </span>
        ) : (
          <DeleteAccountButton
            account={account}
            liveKeys={live.get(account.id) ?? 0}
            finalFocus={deleteFocus}
          />
        ),
    },
  ]
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
      <DataGrid
        label="Service accounts"
        rows={keys.accounts}
        columns={accountColumns(live, deleteFocus)}
        rowId={accountId}
        rowAttributes={(account) => ({ "data-account-id": account.id })}
      />
    </section>
  )
}
