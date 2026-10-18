import { useId, useRef, useState, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { cn } from "cn"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatDateTime, formatRelative } from "@/lib/time"

import { keyStatus } from "./key-status"
import {
  serviceAccountKeysQuery,
  type ServiceAccount,
  type ServiceAccountKey,
} from "./queries"
import { RevokeKeyButton } from "./revoke-key-button"

type Props = {
  account: ServiceAccount
  onCreateKey: (account: ServiceAccount, trigger: HTMLElement) => void
}

function KeysHeader() {
  return (
    <TableHeader>
      <TableRow className="hover:bg-transparent">
        <TableHead>Name</TableHead>
        <TableHead className="w-36 max-sm:hidden">Key</TableHead>
        <TableHead className="w-32 max-sm:hidden">Created</TableHead>
        <TableHead className="w-32 max-sm:w-24">Last used</TableHead>
        <TableHead className="w-32 max-sm:hidden">Expires</TableHead>
        <TableHead className="w-20 text-right">
          <span className="sr-only">Actions</span>
        </TableHead>
      </TableRow>
    </TableHeader>
  )
}

function Moment({ iso }: { iso: string }) {
  return (
    <time dateTime={iso} title={formatDateTime(iso)}>
      {formatRelative(iso)}
    </time>
  )
}

function ExpiryText({ serviceKey }: { serviceKey: ServiceAccountKey }) {
  if (serviceKey.expires_at === null) return <>No expiry</>
  return <Moment iso={serviceKey.expires_at} />
}

type RowProps = {
  account: ServiceAccount
  serviceKey: ServiceAccountKey
  finalFocus: RefObject<HTMLElement | null>
}

function KeyRow({ account, serviceKey, finalFocus }: RowProps) {
  const status = keyStatus(serviceKey)
  const inactive = status !== "active"
  return (
    <TableRow className={cn(inactive && "text-muted-foreground")}>
      <TableCell className="whitespace-normal">
        <div className="font-medium wrap-anywhere">{serviceKey.name}</div>
        <div className="flex flex-col text-xs text-muted-foreground sm:hidden">
          <span className="font-mono">{serviceKey.display_prefix}…</span>
          <span>
            Expires: <ExpiryText serviceKey={serviceKey} />
          </span>
        </div>
      </TableCell>
      <TableCell className="font-mono text-xs max-sm:hidden">
        {serviceKey.display_prefix}…
      </TableCell>
      <TableCell className="max-sm:hidden">
        <Moment iso={serviceKey.created_at} />
      </TableCell>
      <TableCell className="whitespace-normal">
        {serviceKey.last_used_at === null ? (
          <span className="text-muted-foreground">Never</span>
        ) : (
          <Moment iso={serviceKey.last_used_at} />
        )}
      </TableCell>
      <TableCell className="max-sm:hidden">
        <ExpiryText serviceKey={serviceKey} />
      </TableCell>
      <TableCell className="text-right">
        {inactive ? (
          <Badge variant="secondary">
            {status === "revoked" ? "Revoked" : "Expired"}
          </Badge>
        ) : (
          <RevokeKeyButton
            account={account}
            serviceKey={serviceKey}
            finalFocus={finalFocus}
          />
        )}
      </TableCell>
    </TableRow>
  )
}

function SkeletonRows() {
  return Array.from({ length: 2 }, (_, index) => (
    <TableRow key={index} className="hover:bg-transparent">
      <TableCell>
        <Skeleton className="h-5 w-32" />
      </TableCell>
      <TableCell className="max-sm:hidden">
        <Skeleton className="h-5 w-24" />
      </TableCell>
      <TableCell className="max-sm:hidden">
        <Skeleton className="h-5 w-20" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-5 w-16" />
      </TableCell>
      <TableCell className="max-sm:hidden">
        <Skeleton className="h-5 w-20" />
      </TableCell>
      <TableCell />
    </TableRow>
  ))
}

export function ServiceAccountSection({ account, onCreateKey }: Props) {
  const headingId = useId()
  const inactiveTableId = useId()
  const createButton = useRef<HTMLButtonElement>(null)
  const [showInactive, setShowInactive] = useState(false)
  const query = useQuery(serviceAccountKeysQuery(account.id))
  const keys = query.data ?? []
  const active = keys.filter((key) => keyStatus(key) === "active")
  const inactive = keys.filter((key) => keyStatus(key) !== "active")

  return (
    <section
      aria-labelledby={headingId}
      className="flex flex-col gap-3 py-6 first:pt-0"
    >
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="flex min-w-0 flex-col gap-0.5">
          <h2 id={headingId} className="text-base font-semibold wrap-anywhere">
            {account.name}
          </h2>
          {account.description !== "" ? (
            <p className="text-sm wrap-anywhere text-muted-foreground">
              {account.description}
            </p>
          ) : null}
        </div>
        <Button
          ref={createButton}
          variant="outline"
          size="sm"
          aria-label={`Create key for ${account.name}`}
          onClick={(event) => onCreateKey(account, event.currentTarget)}
        >
          <PlusIcon aria-hidden="true" />
          Create key
        </Button>
      </div>
      {query.isPending ? (
        <Table
          className="table-fixed"
          aria-label={`Active keys for ${account.name}`}
          aria-busy="true"
        >
          <KeysHeader />
          <TableBody>
            <SkeletonRows />
          </TableBody>
        </Table>
      ) : query.isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load keys for {account.name}</AlertTitle>
          <AlertDescription>
            <p>{query.error.message}</p>
            <Button
              variant="outline"
              size="sm"
              className="text-foreground"
              disabled={query.isFetching}
              focusableWhenDisabled
              aria-busy={query.isFetching}
              onClick={() => void query.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        <>
          {active.length === 0 ? (
            <p className="text-sm text-muted-foreground">No active keys.</p>
          ) : (
            <Table
              className="table-fixed"
              aria-label={`Active keys for ${account.name}`}
            >
              <KeysHeader />
              <TableBody>
                {active.map((key) => (
                  <KeyRow
                    key={key.id}
                    account={account}
                    serviceKey={key}
                    finalFocus={createButton}
                  />
                ))}
              </TableBody>
            </Table>
          )}
          {inactive.length > 0 ? (
            <>
              <div>
                <Button
                  variant="link"
                  size="sm"
                  className="px-0"
                  aria-expanded={showInactive}
                  aria-controls={inactiveTableId}
                  onClick={() => setShowInactive((value) => !value)}
                >
                  {showInactive
                    ? "Hide revoked and expired keys"
                    : `Show ${inactive.length} revoked or expired ${inactive.length === 1 ? "key" : "keys"}`}
                </Button>
              </div>
              {showInactive ? (
                <Table
                  id={inactiveTableId}
                  className="table-fixed"
                  aria-label={`Revoked and expired keys for ${account.name}`}
                >
                  <KeysHeader />
                  <TableBody>
                    {inactive.map((key) => (
                      <KeyRow
                        key={key.id}
                        account={account}
                        serviceKey={key}
                        finalFocus={createButton}
                      />
                    ))}
                  </TableBody>
                </Table>
              ) : null}
            </>
          ) : null}
        </>
      )}
    </section>
  )
}
