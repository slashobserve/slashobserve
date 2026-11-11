import { useState } from "react"
import { Link } from "@tanstack/react-router"

import { ApiError } from "@/api/errors"
import { LoadProblem } from "@/components/load-problem"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { roleLabels } from "@/lib/roles"
import { sentence } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import {
  clientText,
  clientTextAtStart,
  labelText,
  returnUrl,
} from "./consent-text"
import {
  useAnswerConnection,
  useConnectionRequest,
  type Answer,
  type ConnectionRequest,
} from "./queries"

const unopenableText =
  "SlashObserve got an address back that this page will not open. Start connecting again from your agent."

function isGone(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

export function ConsentPage({ requestId }: { requestId: string }) {
  const request = useConnectionRequest(requestId)
  const answer = useAnswerConnection(requestId)
  const [unopenable, setUnopenable] = useState(false)

  function respond(choice: Answer) {
    setUnopenable(false)
    answer.mutate(choice, {
      onSuccess: (redirectTo) => {
        const target = returnUrl(redirectTo)
        if (target === null) setUnopenable(true)
        else window.location.assign(target)
      },
    })
  }

  return (
    <main className="flex flex-1 flex-col items-center justify-center p-6">
      <div className="w-full max-w-[480px] rounded-card border bg-background p-6">
        {isGone(request.error) || isGone(answer.error) ? (
          <Gone />
        ) : request.isError ? (
          <LoadProblem
            title="Could not load this request"
            detail={loadProblemText(request.error)}
            retrying={request.isFetching}
            onRetry={() => void request.refetch()}
          />
        ) : request.isPending ? (
          <Pending />
        ) : (
          <Consent
            request={request.data}
            busy={answer.isPending || answer.isSuccess}
            answered={answer.isSuccess && !unopenable ? answer.variables : null}
            problem={unopenable ? unopenableText : null}
            error={answer.error}
            onAnswer={respond}
          />
        )}
      </div>
    </main>
  )
}

function Gone() {
  return (
    <div className="flex flex-col gap-2">
      <h1 className="text-lg font-semibold tracking-tight">
        This request is no longer open
      </h1>
      <p className="text-sm text-muted-foreground">
        This request expired or was already answered. Start connecting again
        from your agent.
      </p>
    </div>
  )
}

function Pending() {
  return (
    <div
      className="flex flex-col gap-3"
      role="status"
      aria-label="Loading request"
    >
      <Skeleton className="h-6 w-3/4" />
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-4 w-5/6" />
    </div>
  )
}

type ConsentProps = {
  request: ConnectionRequest
  busy: boolean
  answered: Answer | null
  problem: string | null
  error: unknown
  onAnswer: (choice: Answer) => void
}

function Consent({
  request,
  busy,
  answered,
  problem,
  error,
  onAnswer,
}: ConsentProps) {
  const { client, person } = request
  const start = clientTextAtStart(client.name)
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-lg font-semibold tracking-tight wrap-anywhere">
        {`Connect ${clientText(client.name)} to SlashObserve`}
      </h1>
      <div className="flex flex-col gap-2 text-sm wrap-anywhere">
        <p>
          {`${start} will act as you, ${person.name} (${person.email}), with your role (${roleLabels[person.role]}) and your folder access. It can read and change everything you can, and what it changes shows as "${labelText(person.name, client.name)}".`}
        </p>
        <p>If your role or folder access changes, so does what it can do.</p>
        <p>{`After you approve, your browser returns to ${client.redirect_host}.`}</p>
      </div>
      {client.local ? (
        <Alert>
          <AlertDescription>
            {`${start} receives this approval on this computer. Approve only if you just started connecting it yourself: any program on this computer can ask under any name.`}
          </AlertDescription>
        </Alert>
      ) : null}
      {request.replaces_grant ? (
        <p className="text-sm">
          You connected this client before. Approving replaces that connection.
        </p>
      ) : null}
      <p className="text-xs text-muted-foreground">
        SlashObserve cannot verify the name a client gives. You can disconnect
        it at any time in Settings.
      </p>
      <AnswerProblem error={error} problem={problem} />
      <div className="flex justify-end gap-2">
        <Button
          variant="outline"
          disabled={busy}
          onClick={() => onAnswer("deny")}
        >
          Deny
        </Button>
        <Button disabled={busy} onClick={() => onAnswer("approve")}>
          Approve
        </Button>
      </div>
      {answered === null ? null : (
        <p role="status" className="text-xs text-muted-foreground">
          {`${answered === "approve" ? "Approved" : "Denied"}. Returning to ${client.redirect_host}.`}
        </p>
      )}
    </div>
  )
}

function AnswerProblem({
  error,
  problem,
}: {
  error: unknown
  problem: string | null
}) {
  if (problem !== null) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{problem}</AlertDescription>
      </Alert>
    )
  }
  if (error === null || error === undefined) return null
  if (error instanceof ApiError && error.code === "too_many_grants") {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          <p>{sentence(error.message)}</p>
          <Link
            to="/settings"
            hash="connected-agents"
            className="underline underline-offset-3"
          >
            Open Settings
          </Link>
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert variant="destructive">
      <AlertDescription>{loadProblemText(error)}</AlertDescription>
    </Alert>
  )
}
