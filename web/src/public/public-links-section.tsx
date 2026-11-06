import { useId, useState, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { CopyButton } from "@/components/copy-button"
import { LoadProblem } from "@/components/load-problem"
import { RelativeTime } from "@/components/relative-time"
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
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { formatDateTime } from "@/lib/time"
import { loadProblemText } from "@/service-accounts/queries"
import type { Variable } from "@/spec"

import {
  expiryChoices,
  expiryFor,
  frozenSelections,
  linkStatus,
  modeChoices,
  modeLabel,
  type ExpiryChoice,
  type PublicLink,
} from "./public-links"
import {
  publicLinksQuery,
  useCreatePublicLink,
  useRevokePublicLink,
} from "./queries"
import type { PublicMode } from "./supported"

type Props = {
  dashboardId: string
  variables: readonly Variable[] | undefined
  vars: Record<string, string[]>
}

export function PublicLinksSection({ dashboardId, variables, vars }: Props) {
  const headingId = useId()
  const links = useQuery(publicLinksQuery(dashboardId))
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-2.5">
      <h3 id={headingId} className="text-[13px] font-semibold">
        Public links
      </h3>
      <p className="text-xs text-muted-foreground">
        Anyone with a public link can view this dashboard without signing in. It
        always shows the latest version, and its queries stay on the server.
      </p>
      <CreatePublicLink
        dashboardId={dashboardId}
        variables={variables}
        vars={vars}
      />
      {links.data !== undefined ? (
        links.data.length === 0 ? null : (
          <ul className="rounded-card border">
            {links.data.map((link) => (
              <PublicLinkRow
                key={link.id}
                dashboardId={dashboardId}
                link={link}
              />
            ))}
          </ul>
        )
      ) : links.isError ? (
        <LoadProblem
          title="Could not load public links"
          detail={loadProblemText(links.error)}
          retrying={links.isFetching}
          onRetry={() => void links.refetch()}
        />
      ) : (
        <Skeleton className="h-16 w-full animate-none motion-safe:animate-pulse" />
      )}
    </section>
  )
}

function CreatePublicLink({ dashboardId, variables, vars }: Props) {
  const ids = useId()
  const create = useCreatePublicLink(dashboardId)
  const [mode, setMode] = useState<PublicMode>("frozen")
  const [expiry, setExpiry] = useState<ExpiryChoice>("none")
  const [date, setDate] = useState("")
  const [problem, setProblem] = useState<string | null>(null)
  const [createdUrl, setCreatedUrl] = useState<string | null>(null)

  function submit(event: FormEvent) {
    event.preventDefault()
    if (create.isPending) return
    const chosen = expiryFor(expiry, date)
    if (!chosen.ok) {
      setProblem(chosen.problem)
      return
    }
    setProblem(null)
    setCreatedUrl(null)
    create.mutate(
      {
        variables: mode,
        selections: mode === "frozen" ? frozenSelections(variables, vars) : {},
        expires_at: chosen.expiresAt,
      },
      {
        onSuccess: (created) => {
          setCreatedUrl(created.url)
          create.reset()
        },
        onError: (error) => setProblem(loadProblemText(error)),
      }
    )
  }

  return (
    <form
      noValidate
      onSubmit={submit}
      className="flex flex-col gap-2"
      aria-label="Create public link"
    >
      <div className="flex gap-1.5">
        <Select
          items={modeChoices}
          value={mode}
          onValueChange={(next) => {
            if (next !== null) setMode(next)
          }}
        >
          <SelectTrigger
            id={`${ids}-mode`}
            aria-label="Variables"
            className="min-w-0 flex-1"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {modeChoices.map((choice) => (
              <SelectItem key={choice.value} value={choice.value}>
                {choice.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          items={expiryChoices}
          value={expiry}
          onValueChange={(next) => {
            if (next !== null) setExpiry(next)
          }}
        >
          <SelectTrigger
            id={`${ids}-expiry`}
            aria-label="Expires"
            className="min-w-0 flex-1"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {expiryChoices.map((choice) => (
              <SelectItem key={choice.value} value={choice.value}>
                {choice.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {expiry === "date" ? (
        <div className="flex flex-col gap-1">
          <label htmlFor={`${ids}-date`} className="text-xs">
            Stops working after
          </label>
          <Input
            id={`${ids}-date`}
            type="date"
            value={date}
            onChange={(event) => setDate(event.target.value)}
          />
        </div>
      ) : null}
      {problem !== null ? (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      ) : null}
      <div>
        <Button
          type="submit"
          variant="outline"
          size="sm"
          disabled={create.isPending}
          focusableWhenDisabled
          aria-busy={create.isPending}
        >
          {create.isPending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="size-3.5 motion-safe:animate-spin"
            />
          ) : null}
          Create public link
        </Button>
      </div>
      {createdUrl !== null ? <CreatedLink url={createdUrl} /> : null}
    </form>
  )
}

function CreatedLink({ url }: { url: string }) {
  const inputId = useId()
  const hintId = useId()
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex gap-1.5">
        <label htmlFor={inputId} className="sr-only">
          Public link
        </label>
        <Input
          id={inputId}
          readOnly
          value={url}
          aria-describedby={hintId}
          onFocus={(event) => event.currentTarget.select()}
          className="flex-1 font-mono text-xs"
        />
        <CopyButton text={url} label="Copy public link" icon={false}>
          Copy
        </CopyButton>
      </div>
      <p id={hintId} className="text-xs text-muted-foreground">
        Copy it now. It is not shown again.
      </p>
    </div>
  )
}

function PublicLinkRow({
  dashboardId,
  link,
}: {
  dashboardId: string
  link: PublicLink
}) {
  const status = linkStatus(link)
  return (
    <li className="flex items-start justify-between gap-3 border-b px-3 py-2 text-[13px] last:border-b-0">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-mono text-xs">{link.prefix}</span>
          <span>{modeLabel(link.variables)}</span>
          <span className="text-muted-foreground">
            {link.expires_at === null ? (
              "No expiry"
            ) : (
              <>Expires {formatDateTime(link.expires_at)}</>
            )}
          </span>
        </span>
        {status !== null ? (
          <span className="text-xs text-destructive">{status}</span>
        ) : null}
        <span className="text-xs text-muted-foreground">
          Created by {link.created_by.name}{" "}
          <RelativeTime iso={link.created_at} />, last viewed{" "}
          <RelativeTime iso={link.last_viewed_at} fallback="never" />
        </span>
      </div>
      <RevokePublicLink dashboardId={dashboardId} link={link} />
    </li>
  )
}

function RevokePublicLink({
  dashboardId,
  link,
}: {
  dashboardId: string
  link: PublicLink
}) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const revoke = useRevokePublicLink(dashboardId)

  function changeOpen(next: boolean) {
    if (!next && revoke.isPending) return
    setOpen(next)
    if (!next) {
      setProblem(null)
      revoke.reset()
    }
  }

  function confirm() {
    setProblem(null)
    revoke.mutate(link.id, {
      onSuccess: () => setOpen(false),
      onError: (error) => {
        if (error instanceof ApiError && error.status === 404) setOpen(false)
        else setProblem(loadProblemText(error))
      },
    })
  }

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="px-3 font-normal"
        aria-label={`Revoke public link ${link.prefix}`}
        onClick={() => setOpen(true)}
      >
        Revoke
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke this public link?</AlertDialogTitle>
            <AlertDialogDescription>
              Anyone opening the link that starts with{" "}
              <span className="font-mono text-xs">{link.prefix}</span> sees that
              it no longer works. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={revoke.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={revoke.isPending}
              focusableWhenDisabled
              aria-busy={revoke.isPending}
              onClick={confirm}
            >
              {revoke.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Revoke link
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
