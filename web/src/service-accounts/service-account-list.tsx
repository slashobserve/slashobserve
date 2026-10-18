import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

import {
  loadProblemText,
  serviceAccountsQuery,
  type ServiceAccount,
} from "./queries"
import { ServiceAccountSection } from "./service-account-section"

type Props = {
  labelledBy: string
  onCreateKey: (account: ServiceAccount | null, trigger: HTMLElement) => void
}

export function ServiceAccountList({ labelledBy, onCreateKey }: Props) {
  const query = useQuery(serviceAccountsQuery())

  if (query.isPending) {
    return (
      <div className="flex flex-col divide-y" aria-busy="true">
        {[0, 1].map((index) => (
          <div key={index} className="flex flex-col gap-3 py-6 first:pt-0">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-16 w-full" />
          </div>
        ))}
      </div>
    )
  }

  if (query.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Could not load service accounts</AlertTitle>
        <AlertDescription>
          <p>{loadProblemText(query.error)}</p>
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
    )
  }

  if (query.data.length === 0) {
    return (
      <section className="flex flex-col items-center gap-2 py-16 text-center">
        <h2 className="text-lg font-semibold">No service accounts yet</h2>
        <p className="text-sm text-muted-foreground">
          Create one to get a key for the observe CLI or an MCP client.
        </p>
        <Button onClick={(event) => onCreateKey(null, event.currentTarget)}>
          New service account
        </Button>
      </section>
    )
  }

  return (
    <>
      <div
        role="region"
        aria-labelledby={labelledBy}
        className="flex flex-col divide-y"
      >
        {query.data.map((account) => (
          <ServiceAccountSection
            key={account.id}
            account={account}
            onCreateKey={onCreateKey}
          />
        ))}
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        Last used times update at most every 5 minutes.
      </p>
    </>
  )
}
