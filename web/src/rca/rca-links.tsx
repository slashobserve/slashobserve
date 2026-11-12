import { useEffect, useId, useRef, useState, type FormEvent } from "react"
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
import {
  expiryChoices,
  expiryFor,
  type ExpiryChoice,
} from "@/public/public-links"
import { formatDateTime } from "@/lib/time"
import { loadProblemText } from "@/service-accounts/queries"

import {
  rcaLinksQuery,
  useCreateRcaLink,
  useRevokeRcaLink,
  type RcaLink,
} from "./queries"

type Props = { observationId: string; revision: number }

function statusText(link: RcaLink): string {
  if (link.state === "expired") return "Expired"
  if (link.expires_at === null) return "Active, no expiry"
  return `Active, expires ${formatDateTime(link.expires_at)}`
}

export function RcaLinks({ observationId, revision }: Props) {
  const links = useQuery(rcaLinksQuery(observationId, revision))
  const [urls, setUrls] = useState<Record<string, string>>({})
  const [latest, setLatest] = useState<string | null>(null)

  return (
    <div className="flex flex-col gap-2.5">
      <CreateRcaLink
        observationId={observationId}
        revision={revision}
        latest={latest}
        onCreated={(id, url) => {
          setUrls((known) => ({ ...known, [id]: url }))
          setLatest(url)
        }}
      />
      {links.data !== undefined ? (
        links.data.length === 0 ? null : (
          <ul className="rounded-card border">
            {links.data.map((link) => (
              <RcaLinkRow
                key={link.id}
                observationId={observationId}
                revision={revision}
                link={link}
                url={urls[link.id]}
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
    </div>
  )
}

function CreateRcaLink({
  observationId,
  revision,
  latest,
  onCreated,
}: Props & {
  latest: string | null
  onCreated: (linkId: string, url: string) => void
}) {
  const ids = useId()
  const create = useCreateRcaLink(observationId, revision)
  const [expiry, setExpiry] = useState<ExpiryChoice>("none")
  const [date, setDate] = useState("")
  const [problem, setProblem] = useState<string | null>(null)

  function submit(event: FormEvent) {
    event.preventDefault()
    if (create.isPending) return
    const chosen = expiryFor(expiry, date)
    if (!chosen.ok) {
      setProblem(chosen.problem)
      return
    }
    setProblem(null)
    create.mutate(
      { expires_at: chosen.expiresAt },
      {
        onSuccess: (created) => {
          onCreated(created.link.id, created.url)
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
        <Button
          type="submit"
          size="sm"
          className="h-8"
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
          Create link
        </Button>
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
      {latest !== null ? <CreatedLink url={latest} /> : null}
    </form>
  )
}

function CreatedLink({ url }: { url: string }) {
  const inputId = useId()
  const hintId = useId()
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => input.current?.focus(), [url])
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex gap-1.5">
        <label htmlFor={inputId} className="sr-only">
          Public link
        </label>
        <Input
          ref={input}
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

function RcaLinkRow({
  observationId,
  revision,
  link,
  url,
}: {
  observationId: string
  revision: number
  link: RcaLink
  url: string | undefined
}) {
  return (
    <li className="flex items-start justify-between gap-3 border-b px-3 py-2 text-[13px] last:border-b-0">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-mono text-xs">{link.prefix}</span>
          <span
            className={
              link.state === "expired"
                ? "text-destructive"
                : "text-muted-foreground"
            }
          >
            {statusText(link)}
          </span>
        </span>
        <span className="text-xs text-muted-foreground">
          Created by {link.created_by.name}{" "}
          <RelativeTime iso={link.created_at} />, last viewed{" "}
          <RelativeTime iso={link.last_viewed_at} fallback="never" />
        </span>
      </div>
      <div className="flex flex-none items-center gap-1">
        {url === undefined ? null : (
          <CopyButton
            text={url}
            label={`Copy public link ${link.prefix}`}
            icon={false}
            variant="ghost"
            size="sm"
          >
            Copy link
          </CopyButton>
        )}
        <RevokeRcaLink
          observationId={observationId}
          revision={revision}
          link={link}
        />
      </div>
    </li>
  )
}

function RevokeRcaLink({
  observationId,
  revision,
  link,
}: {
  observationId: string
  revision: number
  link: RcaLink
}) {
  const [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const revoke = useRevokeRcaLink(observationId, revision)

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
